import Foundation
import Observation

enum ProfileError: Error, Equatable {
    /// The server rejected the request with this message.
    case rejected(String)
    case unexpected
}

/// The trimmed name, or nil when it breaks the server's rule: 1 to 40 Unicode scalars after the trim.
/// The server counts code points, so `String.count` (grapheme clusters) would accept names that it rejects.
func displayName(_ input: String) -> String? {
    let name = input.trimmingCharacters(in: .whitespacesAndNewlines)
    return (1...40).contains(name.unicodeScalars.count) ? name : nil
}

/// The profile of the signed-in user, as the server has it.
@MainActor
@Observable
final class Profile {
    private(set) var me: Components.Schemas.Me?

    private let api: any APIProtocol

    init(api: any APIProtocol) {
        self.api = api
    }

    func load() async throws {
        guard case let .ok(ok) = try await api.getMe(.init()) else {
            throw ProfileError.unexpected
        }
        me = try ok.body.json
    }

    func update(_ change: Components.Schemas.MeUpdate) async throws {
        switch try await api.updateMe(.init(body: .json(change))) {
        case let .ok(ok):
            me = try ok.body.json
        case let .badRequest(bad):
            throw ProfileError.rejected(try bad.body.json.message)
        default:
            throw ProfileError.unexpected
        }
    }

    /// The server sets the consent time, so the profile loads again.
    func giveConsent() async throws {
        guard case .noContent = try await api.giveAiConsent(.init()) else {
            throw ProfileError.unexpected
        }
        try await load()
    }

    func withdrawConsent() async throws {
        guard case .noContent = try await api.withdrawAiConsent(.init()) else {
            throw ProfileError.unexpected
        }
        try await load()
    }
}
