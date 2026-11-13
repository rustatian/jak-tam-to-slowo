import Foundation
import Observation

enum DraftError: Error, Equatable {
    /// The model refused to answer.
    case refused
    /// The model gave no card that the server accepts.
    case invalid
    case unavailable
    /// The server refused the text of the request.
    case badText
}

/// An AI draft of a translation card for the group of the signed-in user.
@MainActor
@Observable
final class CardDraft {
    enum Step: Equatable {
        case idle
        case waiting
        /// The text has more than one common meaning, and the learner picks one.
        case choices([String])
        case card(CardFields)
    }

    private(set) var step = Step.idle

    private let api: any APIProtocol
    private let profile: Profile
    /// The wait before each read of the draft.
    private let pause: Duration

    init(api: any APIProtocol, profile: Profile, pause: Duration = .milliseconds(1500)) {
        self.api = api
        self.profile = profile
        self.pause = pause
    }

    /// Asks for a draft of the text, with a meaning from the choices of an earlier draft of the same text.
    /// After an error the step is the step before the request.
    func request(text: String, meaning: String? = nil) async throws {
        guard let groupId = profile.me?.groupId else { return }
        let before = step
        step = .waiting
        do {
            step = try await draft(groupId, text.trimmingCharacters(in: .whitespacesAndNewlines), meaning)
        } catch {
            step = before
            throw error
        }
    }

    func reset() {
        step = .idle
    }

    private func draft(_ groupId: String, _ text: String, _ meaning: String?) async throws -> Step {
        switch try await api.createDraft(.init(path: .init(groupId: groupId), body: .json(.init(text: text, meaningRu: meaning)))) {
        case let .accepted(accepted):
            let id = try accepted.body.json.id
            // 80 reads with the pause of 1.5 seconds are the limit of 120 seconds for a draft.
            for _ in 0..<80 {
                try await Task.sleep(for: pause)
                guard case let .ok(ok) = try await api.getDraft(.init(path: .init(groupId: groupId, draftId: id))) else {
                    throw DraftError.unavailable
                }
                let draft = try ok.body.json
                switch draft.status {
                case .queued, .running:
                    continue
                case .done:
                    if let choices = draft.choices {
                        return .choices(choices)
                    }
                    guard let card = draft.card else { throw DraftError.invalid }
                    return .card(CardFields(card))
                case .failed:
                    switch draft.error {
                    case .refused:
                        throw DraftError.refused
                    case .invalid:
                        throw DraftError.invalid
                    case .unavailable, nil:
                        throw DraftError.unavailable
                    }
                }
            }
            throw DraftError.unavailable
        case .badRequest:
            throw DraftError.badText
        case .forbidden:
            // The user lost the AI consent or the group. The profile loads again to show the current state.
            try await profile.load()
            return .idle
        default:
            throw DraftError.unavailable
        }
    }
}
