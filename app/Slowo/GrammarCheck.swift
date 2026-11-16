import Foundation
import Observation

enum GrammarCheckError: Error, Equatable {
    /// The model refused to answer.
    case refused
    /// The model gave no cards that the server accepts.
    case invalid
    case unavailable
    /// The server refused the text of the request.
    case badText
}

/// A grammar check of a text for the group of the signed-in user, and the cards of the check before the save.
@MainActor
@Observable
final class GrammarCheck {
    private(set) var waiting = false
    /// True after a check that found no mistake, until the next request or `reset`.
    private(set) var clean = false
    /// The cards of the last check that are not saved and not removed. The check screen edits them.
    var cards: [GrammarFields] = []

    private let api: any APIProtocol
    private let profile: Profile
    private let collection: Cards
    /// The wait before each read of the check.
    private let pause: Duration
    /// The group ID, the sent text and the ID of the last check that has no final answer.
    /// A check costs time and money, so a request with the same group and text reads this check.
    private var kept: (groupId: String, text: String, id: String)?

    init(api: any APIProtocol, profile: Profile, collection: Cards, pause: Duration = .milliseconds(1500)) {
        self.api = api
        self.profile = profile
        self.collection = collection
        self.pause = pause
    }

    /// Sends the text and reads the check until its final answer. A request with the text of a check that has no final answer reads that check.
    func request(text: String) async throws {
        guard let groupId = profile.me?.groupId else { return }
        // The server accepts the text on one line.
        let text = trimmed(text.split(whereSeparator: \.isNewline).joined(separator: " "))
        waiting = true
        clean = false
        defer { waiting = false }

        let id: String
        if let kept, kept.groupId == groupId, kept.text == text {
            id = kept.id
        } else {
            switch try await api.createGrammarCheck(.init(path: .init(groupId: groupId), body: .json(.init(text: text)))) {
            case let .accepted(accepted):
                id = try accepted.body.json.id
                kept = (groupId, text, id)
            case .badRequest:
                throw GrammarCheckError.badText
            case .forbidden:
                // The user lost the AI consent or the group. The profile loads again to show the current state.
                try await profile.load()
                return
            default:
                throw GrammarCheckError.unavailable
            }
        }

        // 214 reads with the pause of 1.5 seconds are longer than the limit of 320 seconds for a check.
        for _ in 0..<214 {
            try await Task.sleep(for: pause)
            switch try await api.getGrammarCheck(.init(path: .init(groupId: groupId, checkId: id))) {
            case let .ok(ok):
                let check = try ok.body.json
                switch check.status {
                case .queued, .running:
                    continue
                case .done:
                    kept = nil
                    cards = try (check.cards ?? []).enumerated().map { position, card in
                        guard let fields = GrammarFields(id: position, card) else { throw GrammarCheckError.invalid }
                        return fields
                    }
                    clean = cards.isEmpty
                    return
                case .failed:
                    kept = nil
                    switch check.error {
                    case .refused:
                        throw GrammarCheckError.refused
                    case .invalid:
                        throw GrammarCheckError.invalid
                    case .unavailable, nil:
                        throw GrammarCheckError.unavailable
                    }
                }
            case .notFound:
                kept = nil
                throw GrammarCheckError.unavailable
            case .forbidden:
                try await profile.load()
                return
            default:
                throw GrammarCheckError.unavailable
            }
        }
        throw GrammarCheckError.unavailable
    }

    /// Takes one card from the list.
    func remove(_ id: Int) {
        cards.removeAll { $0.id == id }
    }

    /// Creates the cards of the list one after the other, first card first. A created card leaves the list at once. An error stops the save and keeps the cards that are left.
    func save() async throws {
        while let first = cards.first {
            // Without a result the user is not a member of the group any more, so no card can be saved.
            guard try await collection.create(first.create) != nil else {
                cards = []
                return
            }
            remove(first.id)
        }
    }

    /// Removes the cards and the result of a check with no mistake.
    func reset() {
        cards = []
        clean = false
    }
}
