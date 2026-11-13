import Foundation
import Observation

enum CardError: Error, Equatable {
    /// The server refused a text of the card.
    case rejected
    /// The card is in the archive.
    case gone
    case unexpected
}

/// The translation cards of the group of the signed-in user for a search text, as the server has them.
@MainActor
@Observable
final class Cards {
    private(set) var cards: [Components.Schemas.Card]?
    /// The trimmed search text of the answer in `cards`. It is empty when `cards` is nil.
    private(set) var query = ""

    private let api: any APIProtocol
    private let profile: Profile
    /// The group ID and the trimmed search text of the last call.
    private var last: (groupId: String, query: String)?

    init(api: any APIProtocol, profile: Profile) {
        self.api = api
        self.profile = profile
    }

    func load(query: String) async throws {
        // After a move to another group the old cards do not show while the new group loads.
        if profile.me?.groupId != last?.groupId {
            clear()
        }
        guard let groupId = profile.me?.groupId else { return }
        let query = query.trimmingCharacters(in: .whitespacesAndNewlines)
        last = (groupId, query)
        let output = try await api.listCards(.init(path: .init(groupId: groupId), query: .init(q: query.isEmpty ? nil : query)))
        // A later call for another group or another search text started, so this answer is old.
        guard last?.groupId == groupId, last?.query == query else { return }
        switch output {
        case let .ok(ok):
            cards = try ok.body.json.cards.filter { $0.kind == .translation }
            self.query = query
        case .forbidden:
            try await dropGroup()
        default:
            throw CardError.unexpected
        }
    }

    func update(_ id: String, _ change: Components.Schemas.CardUpdate) async throws {
        guard let groupId = profile.me?.groupId else { throw CardError.unexpected }
        switch try await api.updateCard(.init(path: .init(groupId: groupId, cardId: id), body: .json(change))) {
        case .ok:
            try await load(query: query)
        case .badRequest:
            throw CardError.rejected
        case .notFound:
            // Another member archived the card, so its row goes away.
            try await load(query: query)
            throw CardError.gone
        case .forbidden:
            try await dropGroup()
        default:
            throw CardError.unexpected
        }
    }

    func create(_ card: Components.Schemas.CardCreate) async throws {
        guard let groupId = profile.me?.groupId else { throw CardError.unexpected }
        switch try await api.createCard(.init(path: .init(groupId: groupId), body: .json(card))) {
        case .created:
            // The card is stored. An error of this load makes the learner save again, and a second save makes a second card.
            if cards != nil {
                try? await load(query: query)
            }
        case .badRequest:
            throw CardError.rejected
        case .forbidden:
            try await dropGroup()
        default:
            throw CardError.unexpected
        }
    }

    /// Archives a card. A card that is in the archive already is not an error.
    func archive(_ id: String) async throws {
        guard let groupId = profile.me?.groupId else { throw CardError.unexpected }
        switch try await api.archiveCard(.init(path: .init(groupId: groupId, cardId: id))) {
        case .noContent, .notFound:
            try await load(query: query)
        case .forbidden:
            try await dropGroup()
        default:
            throw CardError.unexpected
        }
    }

    /// The user is not a member of the group. The profile loads again to get the current group ID.
    private func dropGroup() async throws {
        try await profile.load()
        clear()
    }

    private func clear() {
        cards = nil
        query = ""
    }
}
