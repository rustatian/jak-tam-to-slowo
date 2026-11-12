import Foundation
import Testing

@testable import Slowo

let inG2 = Components.Schemas.Me(id: "s1", email: "ola@example.com", displayName: "", direction: .ruPl, reminder: false, groupId: "g2")
let naszaPreview = Components.Schemas.InvitationPreview(groupId: "g1", groupName: "Nasza", invitedBy: "Ola")
let linkT1 = URL(string: "https://slowo.rustatian.me/invite/T1")!
let linkT2 = URL(string: "https://slowo.rustatian.me/invite/T2")!

struct TokenCase: Sendable, CustomTestStringConvertible {
    let name: String
    let url: String
    let token: String?

    var testDescription: String { name }
}

@Test(arguments: [
    TokenCase(name: "a link from the email", url: "https://slowo.rustatian.me/invite/MFRGGZDFMZTWQ2LKNNWG23TPOB", token: "MFRGGZDFMZTWQ2LKNNWG23TPOB"),
    TokenCase(name: "another host", url: "https://example.com/invite/MFRGGZDFMZTWQ2LKNNWG23TPOB", token: nil),
    TokenCase(name: "another path", url: "https://slowo.rustatian.me/privacy/ABC234", token: nil),
    TokenCase(name: "no token", url: "https://slowo.rustatian.me/invite/", token: nil),
    TokenCase(name: "a path after the token", url: "https://slowo.rustatian.me/invite/ABC234/more", token: nil),
    TokenCase(name: "a period after the token", url: "https://slowo.rustatian.me/invite/ABC234.", token: nil),
    TokenCase(name: "a letter outside ASCII", url: "https://slowo.rustatian.me/invite/AB%C4%86234", token: nil),
])
func invitationTokenRule(_ c: TokenCase) throws {
    #expect(invitationToken(from: try #require(URL(string: c.url))) == c.token)
}

struct InvitationLinkCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    let action: @Sendable @MainActor (InvitationLink, Profile) async throws -> Void
    /// The calls after the first getMe.
    let calls: [String]
    let inputs: [String]
    let error: InvitationError?
    let token: String?
    let isPresented: Bool
    let preview: Components.Schemas.InvitationPreview?
    let groupId: String?

    var testDescription: String { name }
}

let invitationLinkCases: [InvitationLinkCase] = [
    InvitationLinkCase(
        name: "a link sets the token and shows the sheet",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))]),
        action: { _, _ in },
        calls: [], inputs: [], error: nil, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "another address changes nothing",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))], previewInvitation: [.ok(.init(body: .json(naszaPreview)))]),
        action: { link, _ in
            try await link.loadPreview()
            link.isPresented = false
            link.open(URL(string: "https://slowo.rustatian.me/invite/ABC234.")!)
        },
        calls: ["previewInvitation"], inputs: ["previewInvitation T1"], error: nil, token: "T1", isPresented: false, preview: naszaPreview, groupId: nil
    ),
    InvitationLinkCase(
        name: "the preview of a group invitation",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))], previewInvitation: [.ok(.init(body: .json(naszaPreview)))]),
        action: { link, _ in try await link.loadPreview() },
        calls: ["previewInvitation"], inputs: ["previewInvitation T1"], error: nil, token: "T1", isPresented: true, preview: naszaPreview, groupId: nil
    ),
    InvitationLinkCase(
        name: "a new link replaces the token and the preview",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))], previewInvitation: [.ok(.init(body: .json(naszaPreview)))]),
        action: { link, _ in
            try await link.loadPreview()
            link.open(linkT2)
        },
        calls: ["previewInvitation"], inputs: ["previewInvitation T1"], error: nil, token: "T2", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "the same link again shows the sheet and keeps the preview",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))], previewInvitation: [.ok(.init(body: .json(naszaPreview)))]),
        action: { link, _ in
            try await link.loadPreview()
            link.isPresented = false
            link.open(linkT1)
        },
        calls: ["previewInvitation"], inputs: ["previewInvitation T1"], error: nil, token: "T1", isPresented: true, preview: naszaPreview, groupId: nil
    ),
    InvitationLinkCase(
        name: "a link after the sheet closes starts with no preview",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))], previewInvitation: [.ok(.init(body: .json(naszaPreview)))]),
        action: { link, _ in
            try await link.loadPreview()
            link.forget()
            link.open(linkT1)
        },
        calls: ["previewInvitation"], inputs: ["previewInvitation T1"], error: nil, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "a dead link",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone)))],
            previewInvitation: [.notFound(.init(body: .json(.init(message: "invitation not found"))))]
        ),
        action: { link, _ in try await link.loadPreview() },
        calls: ["previewInvitation"], inputs: ["previewInvitation T1"], error: .notFound, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "too many preview calls",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone)))],
            previewInvitation: [.tooManyRequests(.init(body: .json(.init(message: "Too Many Requests"))))]
        ),
        action: { link, _ in try await link.loadPreview() },
        calls: ["previewInvitation"], inputs: ["previewInvitation T1"], error: .tooManyTries, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "a failed preview",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone)))],
            previewInvitation: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { link, _ in try await link.loadPreview() },
        calls: ["previewInvitation"], inputs: ["previewInvitation T1"], error: .unexpected, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "join",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone))), .ok(.init(body: .json(grouped)))],
            previewInvitation: [.ok(.init(body: .json(naszaPreview)))],
            acceptInvitation: [.noContent(.init())]
        ),
        action: { link, profile in
            try await link.loadPreview()
            try await link.accept(profile: profile)
        },
        calls: ["previewInvitation", "acceptInvitation", "getMe"], inputs: ["previewInvitation T1", "acceptInvitation T1 - -"],
        error: nil, token: nil, isPresented: false, preview: naszaPreview, groupId: "g1"
    ),
    InvitationLinkCase(
        name: "a failed profile load after a join",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone))), .internalServerError(.init(body: .json(.init(message: "internal error"))))],
            acceptInvitation: [.noContent(.init())]
        ),
        action: { link, profile in
            await #expect(throws: ProfileError.unexpected) { try await link.accept(profile: profile) }
        },
        calls: ["acceptInvitation", "getMe"], inputs: ["acceptInvitation T1 - -"], error: nil, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "join of a member of the group",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(grouped)))],
            acceptInvitation: [.conflict(.init(body: .json(.init(message: "already a member of this group"))))]
        ),
        action: { link, profile in try await link.accept(profile: profile) },
        calls: ["acceptInvitation", "getMe"], inputs: ["acceptInvitation T1 - -"], error: nil, token: nil, isPresented: false, preview: nil, groupId: "g1"
    ),
    InvitationLinkCase(
        name: "join from another group asks first",
        answers: .init(
            getMe: [.ok(.init(body: .json(inG2)))],
            acceptInvitation: [.conflict(.init(body: .json(.init(message: "in another group"))))]
        ),
        action: { link, profile in try await link.accept(profile: profile) },
        calls: ["acceptInvitation"], inputs: ["acceptInvitation T1 - -"], error: .inAnotherGroup, token: "T1", isPresented: true, preview: nil, groupId: "g2"
    ),
    InvitationLinkCase(
        name: "a conflict with another message",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone)))],
            acceptInvitation: [.conflict(.init(body: .json(.init(message: "another conflict"))))]
        ),
        action: { link, profile in try await link.accept(profile: profile) },
        calls: ["acceptInvitation"], inputs: ["acceptInvitation T1 - -"], error: .unexpected, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "leave and join",
        answers: .init(getMe: [.ok(.init(body: .json(inG2))), .ok(.init(body: .json(grouped)))], acceptInvitation: [.noContent(.init())]),
        action: { link, profile in try await link.accept(profile: profile, leaveGroup: true) },
        calls: ["acceptInvitation", "getMe"], inputs: ["acceptInvitation T1 - leave"], error: nil, token: nil, isPresented: false, preview: nil, groupId: "g1"
    ),
    InvitationLinkCase(
        name: "a new group",
        answers: .init(getMe: [.ok(.init(body: .json(alone))), .ok(.init(body: .json(grouped)))], acceptInvitation: [.noContent(.init())]),
        action: { link, profile in try await link.accept(profile: profile, groupName: "Nasza") },
        calls: ["acceptInvitation", "getMe"], inputs: ["acceptInvitation T1 Nasza -"], error: nil, token: nil, isPresented: false, preview: nil, groupId: "g1"
    ),
    InvitationLinkCase(
        name: "a rejected group name",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone)))],
            acceptInvitation: [.badRequest(.init(body: .json(.init(message: "name must have 1 to 40 characters"))))]
        ),
        action: { link, profile in try await link.accept(profile: profile, groupName: "Nasza") },
        calls: ["acceptInvitation"], inputs: ["acceptInvitation T1 Nasza -"], error: .badName, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "a 404 after an accept that worked",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone))), .ok(.init(body: .json(grouped)))],
            acceptInvitation: [.notFound(.init(body: .json(.init(message: "invitation not found"))))]
        ),
        action: { link, profile in try await link.accept(profile: profile) },
        calls: ["acceptInvitation", "getMe"], inputs: ["acceptInvitation T1 - -"], error: nil, token: nil, isPresented: false, preview: nil, groupId: "g1"
    ),
    InvitationLinkCase(
        name: "a 404 of a dead invitation",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone))), .ok(.init(body: .json(alone)))],
            acceptInvitation: [.notFound(.init(body: .json(.init(message: "invitation not found"))))]
        ),
        action: { link, profile in try await link.accept(profile: profile) },
        calls: ["acceptInvitation", "getMe"], inputs: ["acceptInvitation T1 - -"], error: .notFound, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
    InvitationLinkCase(
        name: "a 404 of a dead invitation in another group",
        answers: .init(
            getMe: [.ok(.init(body: .json(inG2))), .ok(.init(body: .json(inG2)))],
            acceptInvitation: [.notFound(.init(body: .json(.init(message: "invitation not found"))))]
        ),
        action: { link, profile in try await link.accept(profile: profile) },
        calls: ["acceptInvitation", "getMe"], inputs: ["acceptInvitation T1 - -"], error: .notFound, token: "T1", isPresented: true, preview: nil, groupId: "g2"
    ),
    InvitationLinkCase(
        name: "a failed accept",
        answers: .init(
            getMe: [.ok(.init(body: .json(alone)))],
            acceptInvitation: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { link, profile in try await link.accept(profile: profile) },
        calls: ["acceptInvitation"], inputs: ["acceptInvitation T1 - -"], error: .unexpected, token: "T1", isPresented: true, preview: nil, groupId: nil
    ),
]

@MainActor
@Test(arguments: invitationLinkCases)
func invitationLink(_ c: InvitationLinkCase) async throws {
    let api = FakeAPI(c.answers)
    let profile = Profile(api: api)
    try await profile.load()
    let link = InvitationLink(api: api)
    link.open(linkT1)

    var thrown: InvitationError?
    do {
        try await c.action(link, profile)
    } catch let error as InvitationError {
        thrown = error
    }

    #expect(thrown == c.error)
    #expect(api.calls == ["getMe"] + c.calls)
    #expect(api.inputs == c.inputs)
    #expect(link.token == c.token)
    #expect(link.isPresented == c.isPresented)
    #expect(link.preview == c.preview)
    #expect(profile.me?.groupId == c.groupId)
}
