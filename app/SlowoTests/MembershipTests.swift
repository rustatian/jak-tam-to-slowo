import Foundation
import Testing

@testable import Slowo

let grouped = Components.Schemas.Me(id: "s1", email: "ola@example.com", displayName: "", direction: .ruPl, reminder: false, groupId: "g1")
let alone = oldMe
let nasza = Components.Schemas.Group(id: "g1", name: "Nasza", createdAt: start)
let ola = Components.Schemas.Member(id: "s1", displayName: "Ola")
let ivan = Components.Schemas.Member(id: "s2", displayName: "Iwan")
let kasia = Components.Schemas.Invitation(
    id: "i1", email: "kasia@example.com", invitedBy: ola, createdAt: start, expiresAt: start.addingTimeInterval(7 * 24 * 3600)
)
let kasia3 = Components.Schemas.Invitation(
    id: "i3", email: "kasia@example.com", invitedBy: ola, createdAt: start, expiresAt: start.addingTimeInterval(7 * 24 * 3600)
)
let bounced = Components.Schemas.Invitation(
    id: "i2", email: "zly@example.com", createdAt: start, expiresAt: start.addingTimeInterval(7 * 24 * 3600), failure: .bounce
)

struct MembershipCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    let action: @Sendable @MainActor (Membership) async throws -> Void
    let calls: [String]
    let inputs: [String]
    let error: GroupError?
    let group: Components.Schemas.Group?
    let members: [Components.Schemas.Member]
    let invitations: [Components.Schemas.Invitation]

    var testDescription: String { name }
}

let membershipCases: [MembershipCase] = [
    MembershipCase(
        name: "load reads the group and the members",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations"], inputs: [], error: nil, group: nasza, members: [ola, ivan], invitations: []
    ),
    MembershipCase(
        name: "without a group nothing loads",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))]),
        action: { try await $0.load() },
        calls: ["getMe"], inputs: [], error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a removed member sees no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "getMe"], inputs: [], error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a failed load shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup"], inputs: [], error: .unexpected, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a removal between the two reads shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers", "getMe"], inputs: [], error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a failed member read shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers"], inputs: [], error: .unexpected, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a failed reload keeps the shown group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza))), .internalServerError(.init(body: .json(.init(message: "internal error"))))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.load()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "getGroup"], inputs: [], error: .unexpected, group: nasza, members: [ola, ivan], invitations: []
    ),
    MembershipCase(
        name: "create joins the new group",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone))), .ok(.init(body: .json(grouped)))],
            createGroup: [.created(.init(body: .json(nasza)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.create(name: "Nasza")
            try await $0.load()
        },
        calls: ["getMe", "createGroup", "getMe", "getGroup", "listGroupMembers", "listInvitations"], inputs: ["createGroup Nasza"],
        error: nil, group: nasza, members: [ola], invitations: []
    ),
    MembershipCase(
        name: "create after a group from another device",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone))), .ok(.init(body: .json(grouped)))],
            createGroup: [.conflict(.init(body: .json(.init(message: "already in a group"))))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.create(name: "Nasza")
            try await $0.load()
        },
        calls: ["getMe", "createGroup", "getMe", "getGroup", "listGroupMembers", "listInvitations"], inputs: ["createGroup Nasza"],
        error: nil, group: nasza, members: [ola], invitations: []
    ),
    MembershipCase(
        name: "a rejected name keeps the state",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone)))],
            createGroup: [.badRequest(.init(body: .json(.init(message: "name must have 1 to 40 characters"))))]
        ),
        action: { try await $0.create(name: "Nasza") },
        calls: ["getMe", "createGroup"], inputs: ["createGroup Nasza"], error: .badName, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "rename stores the new name",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            updateGroup: [.ok(.init(body: .json(.init(id: "g1", name: "Nowa", createdAt: start))))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.rename("Nowa")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "updateGroup"], inputs: ["updateGroup g1 Nowa"],
        error: nil, group: .init(id: "g1", name: "Nowa", createdAt: start), members: [ola, ivan], invitations: []
    ),
    MembershipCase(
        name: "a rejected new name keeps the group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            updateGroup: [.badRequest(.init(body: .json(.init(message: "name must have 1 to 40 characters"))))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.rename("Nowa")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "updateGroup"], inputs: ["updateGroup g1 Nowa"],
        error: .badName, group: nasza, members: [ola, ivan], invitations: []
    ),
    MembershipCase(
        name: "rename after a removal shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            updateGroup: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.rename("Nowa")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "updateGroup", "getMe"], inputs: ["updateGroup g1 Nowa"],
        error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "remove reloads the members",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan])))), .ok(.init(body: .json(.init(members: [ola]))))],
            removeGroupMember: [.noContent(.init())],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.remove("s2")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "removeGroupMember", "listGroupMembers"], inputs: ["removeGroupMember g1 s2"],
        error: nil, group: nasza, members: [ola], invitations: []
    ),
    MembershipCase(
        name: "remove of a member who is gone",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan])))), .ok(.init(body: .json(.init(members: [ola]))))],
            removeGroupMember: [.notFound(.init(body: .json(.init(message: "member not found"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.remove("s2")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "removeGroupMember", "listGroupMembers"], inputs: ["removeGroupMember g1 s2"],
        error: nil, group: nasza, members: [ola], invitations: []
    ),
    MembershipCase(
        name: "remove after a removal shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.remove("s2")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "removeGroupMember", "getMe"], inputs: ["removeGroupMember g1 s2"],
        error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "leave clears the group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.noContent(.init())],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.leave()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "removeGroupMember", "getMe"], inputs: ["removeGroupMember g1 s1"],
        error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "leave of a member who is gone",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.notFound(.init(body: .json(.init(message: "member not found"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.leave()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "removeGroupMember", "getMe"], inputs: ["removeGroupMember g1 s1"],
        error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "leave after a removal shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.leave()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "removeGroupMember", "getMe"], inputs: ["removeGroupMember g1 s1"],
        error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a failed leave keeps the group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.internalServerError(.init(body: .json(.init(message: "internal error"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: []))))]
        ),
        action: {
            try await $0.load()
            try await $0.leave()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "removeGroupMember"], inputs: ["removeGroupMember g1 s1"],
        error: .unexpected, group: nasza, members: [ola, ivan], invitations: []
    ),
    MembershipCase(
        name: "load reads the invitations",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia, bounced]))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations"], inputs: [],
        error: nil, group: nasza, members: [ola, ivan], invitations: [kasia, bounced]
    ),
    MembershipCase(
        name: "a removal before the invitation read shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "getMe"], inputs: [],
        error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a failed invitation read shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations"], inputs: [],
        error: .unexpected, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "invite reads the list again",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            createInvitation: [.created(.init(body: .json(kasia3)))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia])))), .ok(.init(body: .json(.init(invitations: [kasia3]))))]
        ),
        action: {
            try await $0.load()
            try await $0.invite(email: "kasia@example.com")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "createInvitation", "listInvitations"], inputs: ["createInvitation g1 kasia@example.com"],
        error: nil, group: nasza, members: [ola, ivan], invitations: [kasia3]
    ),
    MembershipCase(
        name: "an address that the server rejects",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            createInvitation: [.badRequest(.init(body: .json(.init(message: "email is not valid"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia]))))]
        ),
        action: {
            try await $0.load()
            try await $0.invite(email: "kasia")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "createInvitation"], inputs: ["createInvitation g1 kasia"],
        error: .badEmail, group: nasza, members: [ola, ivan], invitations: [kasia]
    ),
    MembershipCase(
        name: "the limit of the day",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            createInvitation: [.tooManyRequests(.init(body: .json(.init(message: "too many invitations today"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia]))))]
        ),
        action: {
            try await $0.load()
            try await $0.invite(email: "kasia@example.com")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "createInvitation"], inputs: ["createInvitation g1 kasia@example.com"],
        error: .tooManyInvitations, group: nasza, members: [ola, ivan], invitations: [kasia]
    ),
    MembershipCase(
        name: "an email that was not sent",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            createInvitation: [.badGateway(.init(body: .json(.init(message: "the invitation email was not sent"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia]))))]
        ),
        action: {
            try await $0.load()
            try await $0.invite(email: "kasia@example.com")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "createInvitation"], inputs: ["createInvitation g1 kasia@example.com"],
        error: .emailNotSent, group: nasza, members: [ola, ivan], invitations: [kasia]
    ),
    MembershipCase(
        name: "invite after a removal shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            createInvitation: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia]))))]
        ),
        action: {
            try await $0.load()
            try await $0.invite(email: "kasia@example.com")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "createInvitation", "getMe"], inputs: ["createInvitation g1 kasia@example.com"],
        error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a failed invite keeps the list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            createInvitation: [.internalServerError(.init(body: .json(.init(message: "internal error"))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia]))))]
        ),
        action: {
            try await $0.load()
            try await $0.invite(email: "kasia@example.com")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "createInvitation"], inputs: ["createInvitation g1 kasia@example.com"],
        error: .unexpected, group: nasza, members: [ola, ivan], invitations: [kasia]
    ),
    MembershipCase(
        name: "cancel reads the list again",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia])))), .ok(.init(body: .json(.init(invitations: []))))],
            cancelInvitation: [.noContent(.init())]
        ),
        action: {
            try await $0.load()
            try await $0.cancel("i1")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "cancelInvitation", "listInvitations"], inputs: ["cancelInvitation g1 i1"],
        error: nil, group: nasza, members: [ola, ivan], invitations: []
    ),
    MembershipCase(
        name: "cancel of an invitation that is gone",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia])))), .ok(.init(body: .json(.init(invitations: []))))],
            cancelInvitation: [.notFound(.init(body: .json(.init(message: "invitation not found"))))]
        ),
        action: {
            try await $0.load()
            try await $0.cancel("i1")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "cancelInvitation", "listInvitations"], inputs: ["cancelInvitation g1 i1"],
        error: nil, group: nasza, members: [ola, ivan], invitations: []
    ),
    MembershipCase(
        name: "cancel after a removal shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia]))))],
            cancelInvitation: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: {
            try await $0.load()
            try await $0.cancel("i1")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "cancelInvitation", "getMe"], inputs: ["cancelInvitation g1 i1"],
        error: nil, group: nil, members: [], invitations: []
    ),
    MembershipCase(
        name: "a failed cancel keeps the list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            listInvitations: [.ok(.init(body: .json(.init(invitations: [kasia]))))],
            cancelInvitation: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: {
            try await $0.load()
            try await $0.cancel("i1")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "listInvitations", "cancelInvitation"], inputs: ["cancelInvitation g1 i1"],
        error: .unexpected, group: nasza, members: [ola, ivan], invitations: [kasia]
    ),
]

@MainActor
@Test(arguments: membershipCases)
func membership(_ c: MembershipCase) async throws {
    let api = FakeAPI(c.answers)
    let profile = Profile(api: api)
    try await profile.load()
    let membership = Membership(api: api, profile: profile)

    var thrown: GroupError?
    do {
        try await c.action(membership)
    } catch let error as GroupError {
        thrown = error
    }

    #expect(thrown == c.error)
    #expect(api.calls == c.calls)
    #expect(api.inputs == c.inputs)
    #expect(membership.group == c.group)
    #expect(membership.members == c.members)
    #expect(membership.invitations == c.invitations)
}
