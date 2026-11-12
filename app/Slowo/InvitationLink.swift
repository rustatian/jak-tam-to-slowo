import Foundation
import Observation

enum InvitationError: Error, Equatable {
    case notFound
    case tooManyTries
    /// The user must confirm that they leave their current group.
    case inAnotherGroup
    /// The server rejected the name of the new group.
    case badName
    case unexpected
}

/// The token of a link `https://slowo.rustatian.me/invite/<token>`, or nil for each other URL.
/// The server makes tokens of ASCII letters and digits only.
func invitationToken(from url: URL) -> String? {
    let parts = url.pathComponents
    guard url.host() == "slowo.rustatian.me", parts.count == 3, parts[1] == "invite",
          parts[2].allSatisfy({ $0.isASCII && ($0.isLetter || $0.isNumber) })
    else { return nil }
    return parts[2]
}

/// The invitation link that the app opened. The token stays only in memory.
@MainActor
@Observable
final class InvitationLink {
    private(set) var token: String?
    private(set) var preview: Components.Schemas.InvitationPreview?
    var isPresented = false

    private let api: any APIProtocol

    init(api: any APIProtocol) {
        self.api = api
    }

    /// Shows the invitation of the link. A link with another token replaces the token and its preview.
    func open(_ url: URL) {
        guard let token = invitationToken(from: url) else { return }
        if token != self.token {
            self.token = token
            preview = nil
        }
        isPresented = true
    }

    func loadPreview() async throws {
        guard let token else { return }
        switch try await api.previewInvitation(.init(path: .init(token: token))) {
        case let .ok(ok):
            preview = try ok.body.json
        case .notFound:
            throw InvitationError.notFound
        case .tooManyRequests:
            throw InvitationError.tooManyTries
        default:
            throw InvitationError.unexpected
        }
    }

    /// Joins the group of the invitation, or starts the new group with `groupName`.
    /// `leaveGroup` confirms that the user leaves their current group.
    func accept(profile: Profile, groupName: String? = nil, leaveGroup: Bool = false) async throws {
        guard let token else { throw InvitationError.unexpected }
        let groupId = profile.me?.groupId
        switch try await api.acceptInvitation(.init(path: .init(token: token), body: .json(.init(leaveGroup: leaveGroup ? true : nil, groupName: groupName)))) {
        case .noContent:
            try await profile.load()
            forget()
        case let .conflict(conflict):
            switch try conflict.body.json.message {
            case "already a member of this group":
                try await profile.load()
                forget()
            case "in another group":
                throw InvitationError.inAnotherGroup
            default:
                throw InvitationError.unexpected
            }
        case .badRequest:
            throw InvitationError.badName
        // The server deletes an accepted invitation, so the retry of an accept that worked gets a 404 too.
        // A changed group ID tells that case apart from a dead invitation.
        case .notFound:
            try await profile.load()
            guard profile.me?.groupId != groupId else { throw InvitationError.notFound }
            forget()
        default:
            throw InvitationError.unexpected
        }
    }

    /// Keeps the preview, so the closing sheet keeps its content. The next link clears it in `open(_:)`.
    func forget() {
        token = nil
        isPresented = false
    }
}
