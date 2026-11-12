import Observation

enum GroupError: Error, Equatable {
    /// The server rejected the group name.
    case badName
    case badEmail
    case tooManyInvitations
    case emailNotSent
    case unexpected
}

/// The group of the signed-in user, its members and its pending invitations, as the server has them.
@MainActor
@Observable
final class Membership {
    private(set) var group: Components.Schemas.Group?
    private(set) var members: [Components.Schemas.Member] = []
    private(set) var invitations: [Components.Schemas.Invitation] = []

    private let api: any APIProtocol
    private let profile: Profile

    init(api: any APIProtocol, profile: Profile) {
        self.api = api
        self.profile = profile
    }

    func load() async throws {
        // After a move to another group the old group does not show while the new one loads.
        if group?.id != profile.me?.groupId {
            clear()
        }
        guard let groupId = profile.me?.groupId else { return }
        switch try await api.getGroup(.init(path: .init(groupId: groupId))) {
        case let .ok(ok):
            let group = try ok.body.json
            if let members = try await listMembers(groupId), let invitations = try await listInvitations(groupId) {
                self.group = group
                self.members = members
                self.invitations = invitations
            }
        case .forbidden:
            try await dropGroup()
        default:
            throw GroupError.unexpected
        }
    }

    /// The view loads the new group when the group ID of the profile changes.
    func create(name: String) async throws {
        switch try await api.createGroup(.init(body: .json(.init(name: name)))) {
        // A conflict means that the user joined a group on another device.
        case .created, .conflict:
            try await profile.load()
        case .badRequest:
            throw GroupError.badName
        default:
            throw GroupError.unexpected
        }
    }

    func rename(_ name: String) async throws {
        guard let groupId = group?.id else { throw GroupError.unexpected }
        switch try await api.updateGroup(.init(path: .init(groupId: groupId), body: .json(.init(name: name)))) {
        case let .ok(ok):
            group = try ok.body.json
        case .badRequest:
            throw GroupError.badName
        case .forbidden:
            try await dropGroup()
        default:
            throw GroupError.unexpected
        }
    }

    /// Removes another member. A member who is gone already is not an error.
    func remove(_ memberID: String) async throws {
        guard let groupId = group?.id else { throw GroupError.unexpected }
        switch try await api.removeGroupMember(.init(path: .init(groupId: groupId, userId: memberID))) {
        case .noContent, .notFound:
            if let members = try await listMembers(groupId) {
                self.members = members
            }
        case .forbidden:
            try await dropGroup()
        default:
            throw GroupError.unexpected
        }
    }

    func invite(email: String) async throws {
        guard let groupId = group?.id else { throw GroupError.unexpected }
        switch try await api.createInvitation(.init(path: .init(groupId: groupId), body: .json(.init(email: email)))) {
        // The new invitation replaces a pending one of the same address.
        case .created:
            if let invitations = try await listInvitations(groupId) {
                self.invitations = invitations
            }
        case .badRequest:
            throw GroupError.badEmail
        case .tooManyRequests:
            throw GroupError.tooManyInvitations
        case .badGateway:
            throw GroupError.emailNotSent
        case .forbidden:
            try await dropGroup()
        default:
            throw GroupError.unexpected
        }
    }

    /// Cancels a pending invitation. An invitation that is gone already is not an error.
    func cancel(_ invitationID: String) async throws {
        guard let groupId = group?.id else { throw GroupError.unexpected }
        switch try await api.cancelInvitation(.init(path: .init(groupId: groupId, invitationId: invitationID))) {
        case .noContent, .notFound:
            if let invitations = try await listInvitations(groupId) {
                self.invitations = invitations
            }
        case .forbidden:
            try await dropGroup()
        default:
            throw GroupError.unexpected
        }
    }

    func leave() async throws {
        guard let groupId = group?.id, let userId = profile.me?.id else { throw GroupError.unexpected }
        switch try await api.removeGroupMember(.init(path: .init(groupId: groupId, userId: userId))) {
        // The user is not a member after each of these answers.
        case .noContent, .notFound, .forbidden:
            try await dropGroup()
        default:
            throw GroupError.unexpected
        }
    }

    /// The members, or nil when the user is not a member of the group any more.
    private func listMembers(_ groupId: String) async throws -> [Components.Schemas.Member]? {
        switch try await api.listGroupMembers(.init(path: .init(groupId: groupId))) {
        case let .ok(ok):
            return try ok.body.json.members
        case .forbidden:
            try await dropGroup()
            return nil
        default:
            throw GroupError.unexpected
        }
    }

    /// The pending invitations, or nil when the user is not a member of the group any more.
    private func listInvitations(_ groupId: String) async throws -> [Components.Schemas.Invitation]? {
        switch try await api.listInvitations(.init(path: .init(groupId: groupId))) {
        case let .ok(ok):
            return try ok.body.json.invitations
        case .forbidden:
            try await dropGroup()
            return nil
        default:
            throw GroupError.unexpected
        }
    }

    /// The user is not a member of the group. The profile loads again to get the current group ID.
    private func dropGroup() async throws {
        try await profile.load()
        clear()
    }

    private func clear() {
        group = nil
        members = []
        invitations = []
    }
}
