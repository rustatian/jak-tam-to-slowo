import Foundation
import Testing

@testable import Slowo

let grouped = Components.Schemas.Me(id: "s1", email: "ola@example.com", displayName: "", direction: .ruPl, reminder: false, groupId: "g1")
let alone = oldMe
let nasza = Components.Schemas.Group(id: "g1", name: "Nasza", createdAt: start)
let ola = Components.Schemas.Member(id: "s1", displayName: "Ola")
let ivan = Components.Schemas.Member(id: "s2", displayName: "Iwan")

struct MembershipCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    let action: @Sendable @MainActor (Membership) async throws -> Void
    let calls: [String]
    let inputs: [String]
    let error: GroupError?
    let group: Components.Schemas.Group?
    let members: [Components.Schemas.Member]

    var testDescription: String { name }
}

let membershipCases: [MembershipCase] = [
    MembershipCase(
        name: "load reads the group and the members",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers"], inputs: [], error: nil, group: nasza, members: [ola, ivan]
    ),
    MembershipCase(
        name: "without a group nothing loads",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))]),
        action: { try await $0.load() },
        calls: ["getMe"], inputs: [], error: nil, group: nil, members: []
    ),
    MembershipCase(
        name: "a removed member sees no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "getMe"], inputs: [], error: nil, group: nil, members: []
    ),
    MembershipCase(
        name: "a failed load shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup"], inputs: [], error: .unexpected, group: nil, members: []
    ),
    MembershipCase(
        name: "a removal between the two reads shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers", "getMe"], inputs: [], error: nil, group: nil, members: []
    ),
    MembershipCase(
        name: "a failed member read shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.load() },
        calls: ["getMe", "getGroup", "listGroupMembers"], inputs: [], error: .unexpected, group: nil, members: []
    ),
    MembershipCase(
        name: "a failed reload keeps the shown group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza))), .internalServerError(.init(body: .json(.init(message: "internal error"))))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))]
        ),
        action: {
            try await $0.load()
            try await $0.load()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "getGroup"], inputs: [], error: .unexpected, group: nasza, members: [ola, ivan]
    ),
    MembershipCase(
        name: "create joins the new group",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone))), .ok(.init(body: .json(grouped)))],
            createGroup: [.created(.init(body: .json(nasza)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola]))))]
        ),
        action: {
            try await $0.create(name: "Nasza")
            try await $0.load()
        },
        calls: ["getMe", "createGroup", "getMe", "getGroup", "listGroupMembers"], inputs: ["createGroup Nasza"],
        error: nil, group: nasza, members: [ola]
    ),
    MembershipCase(
        name: "create after a group from another device",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone))), .ok(.init(body: .json(grouped)))],
            createGroup: [.conflict(.init(body: .json(.init(message: "already in a group"))))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola]))))]
        ),
        action: {
            try await $0.create(name: "Nasza")
            try await $0.load()
        },
        calls: ["getMe", "createGroup", "getMe", "getGroup", "listGroupMembers"], inputs: ["createGroup Nasza"],
        error: nil, group: nasza, members: [ola]
    ),
    MembershipCase(
        name: "a rejected name keeps the state",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone)))],
            createGroup: [.badRequest(.init(body: .json(.init(message: "name must have 1 to 40 characters"))))]
        ),
        action: { try await $0.create(name: "Nasza") },
        calls: ["getMe", "createGroup"], inputs: ["createGroup Nasza"], error: .badName, group: nil, members: []
    ),
    MembershipCase(
        name: "rename stores the new name",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            updateGroup: [.ok(.init(body: .json(.init(id: "g1", name: "Nowa", createdAt: start))))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))]
        ),
        action: {
            try await $0.load()
            try await $0.rename("Nowa")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "updateGroup"], inputs: ["updateGroup g1 Nowa"],
        error: nil, group: .init(id: "g1", name: "Nowa", createdAt: start), members: [ola, ivan]
    ),
    MembershipCase(
        name: "a rejected new name keeps the group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            updateGroup: [.badRequest(.init(body: .json(.init(message: "name must have 1 to 40 characters"))))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))]
        ),
        action: {
            try await $0.load()
            try await $0.rename("Nowa")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "updateGroup"], inputs: ["updateGroup g1 Nowa"],
        error: .badName, group: nasza, members: [ola, ivan]
    ),
    MembershipCase(
        name: "rename after a removal shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            updateGroup: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))]
        ),
        action: {
            try await $0.load()
            try await $0.rename("Nowa")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "updateGroup", "getMe"], inputs: ["updateGroup g1 Nowa"],
        error: nil, group: nil, members: []
    ),
    MembershipCase(
        name: "remove reloads the members",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan])))), .ok(.init(body: .json(.init(members: [ola]))))],
            removeGroupMember: [.noContent(.init())]
        ),
        action: {
            try await $0.load()
            try await $0.remove("s2")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "removeGroupMember", "listGroupMembers"], inputs: ["removeGroupMember g1 s2"],
        error: nil, group: nasza, members: [ola]
    ),
    MembershipCase(
        name: "remove of a member who is gone",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan])))), .ok(.init(body: .json(.init(members: [ola]))))],
            removeGroupMember: [.notFound(.init(body: .json(.init(message: "member not found"))))]
        ),
        action: {
            try await $0.load()
            try await $0.remove("s2")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "removeGroupMember", "listGroupMembers"], inputs: ["removeGroupMember g1 s2"],
        error: nil, group: nasza, members: [ola]
    ),
    MembershipCase(
        name: "remove after a removal shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: {
            try await $0.load()
            try await $0.remove("s2")
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "removeGroupMember", "getMe"], inputs: ["removeGroupMember g1 s2"],
        error: nil, group: nil, members: []
    ),
    MembershipCase(
        name: "leave clears the group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.noContent(.init())]
        ),
        action: {
            try await $0.load()
            try await $0.leave()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "removeGroupMember", "getMe"], inputs: ["removeGroupMember g1 s1"],
        error: nil, group: nil, members: []
    ),
    MembershipCase(
        name: "leave of a member who is gone",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.notFound(.init(body: .json(.init(message: "member not found"))))]
        ),
        action: {
            try await $0.load()
            try await $0.leave()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "removeGroupMember", "getMe"], inputs: ["removeGroupMember g1 s1"],
        error: nil, group: nil, members: []
    ),
    MembershipCase(
        name: "leave after a removal shows no group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: {
            try await $0.load()
            try await $0.leave()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "removeGroupMember", "getMe"], inputs: ["removeGroupMember g1 s1"],
        error: nil, group: nil, members: []
    ),
    MembershipCase(
        name: "a failed leave keeps the group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getGroup: [.ok(.init(body: .json(nasza)))],
            listGroupMembers: [.ok(.init(body: .json(.init(members: [ola, ivan]))))],
            removeGroupMember: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: {
            try await $0.load()
            try await $0.leave()
        },
        calls: ["getMe", "getGroup", "listGroupMembers", "removeGroupMember"], inputs: ["removeGroupMember g1 s1"],
        error: .unexpected, group: nasza, members: [ola, ivan]
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
}
