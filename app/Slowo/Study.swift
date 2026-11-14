import Foundation
import Observation

enum StudyError: Error, Equatable {
    /// A review got a 404 or a 409: the card is in the archive, or another device answered it first.
    case changed
    /// A review got a 503: the AI model gave no grade.
    case aiUnavailable
    case unexpected
}

/// The study queue of the signed-in learner in the group of the profile, and the card on screen.
@MainActor
@Observable
final class Study {
    private(set) var queue: [Components.Schemas.StudyCard]?
    private(set) var nextDueAt: Date?
    /// The card on screen. A reload keeps it, so it can be absent from `queue`.
    private(set) var card: Components.Schemas.StudyCard?
    /// The feedback for `card`.
    private(set) var review: Components.Schemas.Review?

    private let api: any APIProtocol
    private let profile: Profile
    /// The group ID of the last queue call.
    private var groupId: String?
    /// The body of the last review that got no final answer.
    private var attempt: Components.Schemas.ReviewCreate?

    private let reminder: Reminder

    init(api: any APIProtocol, profile: Profile, reminder: Reminder) {
        self.api = api
        self.profile = profile
        self.reminder = reminder
    }

    func load() async throws {
        // After a move to another group the old queue does not show while the new group loads.
        if profile.me?.groupId != groupId {
            clear()
        }
        guard let groupId = profile.me?.groupId else {
            reminder.clear()
            return
        }
        self.groupId = groupId
        switch try await api.getStudyQueue(.init(path: .init(groupId: groupId))) {
        case let .ok(ok):
            let answer = try ok.body.json
            queue = answer.cards.filter { $0.card.kind == .translation }
            nextDueAt = answer.nextDueAt
            if card == nil {
                card = queue?.first
            }
            await reminder.update(nextDueAt: answer.nextDueAt)
        case .forbidden:
            try await dropGroup()
        default:
            throw StudyError.unexpected
        }
    }

    /// Sends the answer to `card`. An empty text asks for the right answer and counts as a miss.
    func answer(_ text: String) async throws {
        guard let me = profile.me, let groupId = me.groupId, let card else { throw StudyError.unexpected }
        var body = Components.Schemas.ReviewCreate(
            attemptId: attempt?.attemptId ?? "", cardId: card.card.id, progressVersion: card.progressVersion, direction: me.direction,
            answer: text.trimmingCharacters(in: .whitespacesAndNewlines)
        )
        // The same card, progress version, direction and answer are a retry. A retry keeps the attempt ID, so the server counts the answer one time.
        if body != attempt {
            body.attemptId = UUID().uuidString
        }
        attempt = body
        switch try await api.createReview(.init(path: .init(groupId: groupId), body: .json(body))) {
        case let .created(created):
            review = try created.body.json
        case let .ok(ok):
            // The server counted this attempt before and gives the stored grade.
            review = try ok.body.json
        case .notFound, .conflict:
            // Another member archived the card, or another device of the learner answered it first.
            attempt = nil
            try? await load()
            next()
            throw StudyError.changed
        case .forbidden:
            try await dropGroup()
            return
        case .serviceUnavailable:
            throw StudyError.aiUnavailable
        default:
            throw StudyError.unexpected
        }
        attempt = nil
        // The server counted the answer, so an error of this load is no error of the answer.
        try? await load()
    }

    /// Shows the next card of the queue.
    func next() {
        let answered = card?.card.id
        // After a failed reload the old queue still has the answered card.
        queue?.removeAll { $0.card.id == answered }
        review = nil
        card = queue?.first
    }

    /// The learner is not a member of the group. The profile loads again to get the current group ID.
    private func dropGroup() async throws {
        try await profile.load()
        clear()
    }

    private func clear() {
        queue = nil
        nextDueAt = nil
        card = nil
        review = nil
        attempt = nil
    }
}
