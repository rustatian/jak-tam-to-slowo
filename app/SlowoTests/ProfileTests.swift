import Foundation
import OpenAPIRuntime
import Synchronization
import Testing

@testable import Slowo

/// Answers each API call from a queue and keeps the calls in order.
final class FakeAPI: APIProtocol {
    struct Answers: Sendable {
        var getMe: [Operations.GetMe.Output] = []
        var updateMe: [Operations.UpdateMe.Output] = []
        var giveAiConsent: [Operations.GiveAiConsent.Output] = []
        var withdrawAiConsent: [Operations.WithdrawAiConsent.Output] = []
        var createGroup: [Operations.CreateGroup.Output] = []
        var getGroup: [Operations.GetGroup.Output] = []
        var updateGroup: [Operations.UpdateGroup.Output] = []
        var listGroupMembers: [Operations.ListGroupMembers.Output] = []
        var removeGroupMember: [Operations.RemoveGroupMember.Output] = []
        var createInvitation: [Operations.CreateInvitation.Output] = []
        var listInvitations: [Operations.ListInvitations.Output] = []
        var cancelInvitation: [Operations.CancelInvitation.Output] = []
        var previewInvitation: [Operations.PreviewInvitation.Output] = []
        var acceptInvitation: [Operations.AcceptInvitation.Output] = []
    }

    struct Empty: Error {}

    private let state: Mutex<(answers: Answers, calls: [String], updates: [Components.Schemas.MeUpdate], inputs: [String])>

    init(_ answers: Answers) {
        state = Mutex((answers, [], [], []))
    }

    var calls: [String] { state.withLock { $0.calls } }
    var updates: [Components.Schemas.MeUpdate] { state.withLock { $0.updates } }
    var inputs: [String] { state.withLock { $0.inputs } }

    private static func next<T>(_ queue: inout [T]) throws -> T {
        guard !queue.isEmpty else { throw Empty() }
        return queue.removeFirst()
    }

    func getMe(_ input: Operations.GetMe.Input) async throws -> Operations.GetMe.Output {
        try state.withLock { s in
            s.calls.append("getMe")
            return try Self.next(&s.answers.getMe)
        }
    }

    func updateMe(_ input: Operations.UpdateMe.Input) async throws -> Operations.UpdateMe.Output {
        try state.withLock { s in
            s.calls.append("updateMe")
            if case let .json(change) = input.body {
                s.updates.append(change)
            }
            return try Self.next(&s.answers.updateMe)
        }
    }

    func giveAiConsent(_ input: Operations.GiveAiConsent.Input) async throws -> Operations.GiveAiConsent.Output {
        try state.withLock { s in
            s.calls.append("giveAiConsent")
            return try Self.next(&s.answers.giveAiConsent)
        }
    }

    func withdrawAiConsent(_ input: Operations.WithdrawAiConsent.Input) async throws -> Operations.WithdrawAiConsent.Output {
        try state.withLock { s in
            s.calls.append("withdrawAiConsent")
            return try Self.next(&s.answers.withdrawAiConsent)
        }
    }

    func createGroup(_ input: Operations.CreateGroup.Input) async throws -> Operations.CreateGroup.Output {
        try state.withLock { s in
            s.calls.append("createGroup")
            if case let .json(group) = input.body {
                s.inputs.append("createGroup \(group.name)")
            }
            return try Self.next(&s.answers.createGroup)
        }
    }

    func getGroup(_ input: Operations.GetGroup.Input) async throws -> Operations.GetGroup.Output {
        try state.withLock { s in
            s.calls.append("getGroup")
            return try Self.next(&s.answers.getGroup)
        }
    }

    func updateGroup(_ input: Operations.UpdateGroup.Input) async throws -> Operations.UpdateGroup.Output {
        try state.withLock { s in
            s.calls.append("updateGroup")
            if case let .json(change) = input.body {
                s.inputs.append("updateGroup \(input.path.groupId) \(change.name ?? "")")
            }
            return try Self.next(&s.answers.updateGroup)
        }
    }

    func listGroupMembers(_ input: Operations.ListGroupMembers.Input) async throws -> Operations.ListGroupMembers.Output {
        try state.withLock { s in
            s.calls.append("listGroupMembers")
            return try Self.next(&s.answers.listGroupMembers)
        }
    }

    func removeGroupMember(_ input: Operations.RemoveGroupMember.Input) async throws -> Operations.RemoveGroupMember.Output {
        try state.withLock { s in
            s.calls.append("removeGroupMember")
            s.inputs.append("removeGroupMember \(input.path.groupId) \(input.path.userId)")
            return try Self.next(&s.answers.removeGroupMember)
        }
    }

    func createInvitation(_ input: Operations.CreateInvitation.Input) async throws -> Operations.CreateInvitation.Output {
        try state.withLock { s in
            s.calls.append("createInvitation")
            if case let .json(invitation) = input.body {
                s.inputs.append("createInvitation \(input.path.groupId) \(invitation.email)")
            }
            return try Self.next(&s.answers.createInvitation)
        }
    }

    func listInvitations(_ input: Operations.ListInvitations.Input) async throws -> Operations.ListInvitations.Output {
        try state.withLock { s in
            s.calls.append("listInvitations")
            return try Self.next(&s.answers.listInvitations)
        }
    }

    func cancelInvitation(_ input: Operations.CancelInvitation.Input) async throws -> Operations.CancelInvitation.Output {
        try state.withLock { s in
            s.calls.append("cancelInvitation")
            s.inputs.append("cancelInvitation \(input.path.groupId) \(input.path.invitationId)")
            return try Self.next(&s.answers.cancelInvitation)
        }
    }

    func previewInvitation(_ input: Operations.PreviewInvitation.Input) async throws -> Operations.PreviewInvitation.Output {
        try state.withLock { s in
            s.calls.append("previewInvitation")
            s.inputs.append("previewInvitation \(input.path.token)")
            return try Self.next(&s.answers.previewInvitation)
        }
    }

    func acceptInvitation(_ input: Operations.AcceptInvitation.Input) async throws -> Operations.AcceptInvitation.Output {
        try state.withLock { s in
            s.calls.append("acceptInvitation")
            if case let .json(accept) = input.body {
                s.inputs.append("acceptInvitation \(input.path.token) \(accept.groupName ?? "-") \(accept.leaveGroup == true ? "leave" : "-")")
            }
            return try Self.next(&s.answers.acceptInvitation)
        }
    }
}

let oldMe = Components.Schemas.Me(id: "s1", email: "ola@example.com", displayName: "", direction: .ruPl, reminder: false)
let namedMe = Components.Schemas.Me(id: "s1", email: "ola@example.com", displayName: "Ola", direction: .ruPl, reminder: false)
let consentMe = Components.Schemas.Me(id: "s1", email: "ola@example.com", displayName: "", direction: .ruPl, reminder: false, aiConsentAt: start)

struct ProfileCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    let action: @Sendable @MainActor (Profile) async throws -> Void
    let calls: [String]
    let error: ProfileError?
    let me: Components.Schemas.Me

    var testDescription: String { name }
}

let profileCases: [ProfileCase] = [
    ProfileCase(
        name: "update stores the answer",
        answers: .init(getMe: [.ok(.init(body: .json(oldMe)))], updateMe: [.ok(.init(body: .json(namedMe)))]),
        action: { try await $0.update(.init(displayName: "Ola")) },
        calls: ["getMe", "updateMe"], error: nil, me: namedMe
    ),
    ProfileCase(
        name: "a rejected update keeps the old profile",
        answers: .init(
            getMe: [.ok(.init(body: .json(oldMe)))],
            updateMe: [.badRequest(.init(body: .json(.init(message: "displayName must have 1 to 40 characters"))))]
        ),
        action: { try await $0.update(.init(displayName: "x")) },
        calls: ["getMe", "updateMe"], error: .rejected("displayName must have 1 to 40 characters"), me: oldMe
    ),
    ProfileCase(
        name: "giving consent loads the time",
        answers: .init(getMe: [.ok(.init(body: .json(oldMe))), .ok(.init(body: .json(consentMe)))], giveAiConsent: [.noContent(.init())]),
        action: { try await $0.giveConsent() },
        calls: ["getMe", "giveAiConsent", "getMe"], error: nil, me: consentMe
    ),
    ProfileCase(
        name: "withdrawing consent loads the profile without it",
        answers: .init(getMe: [.ok(.init(body: .json(consentMe))), .ok(.init(body: .json(oldMe)))], withdrawAiConsent: [.noContent(.init())]),
        action: { try await $0.withdrawConsent() },
        calls: ["getMe", "withdrawAiConsent", "getMe"], error: nil, me: oldMe
    ),
    ProfileCase(
        name: "a failed consent call keeps the profile",
        answers: .init(
            getMe: [.ok(.init(body: .json(oldMe)))],
            giveAiConsent: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.giveConsent() },
        calls: ["getMe", "giveAiConsent"], error: .unexpected, me: oldMe
    ),
]

@MainActor
@Test(arguments: profileCases)
func profile(_ c: ProfileCase) async throws {
    let api = FakeAPI(c.answers)
    let profile = Profile(api: api)
    try await profile.load()

    var thrown: ProfileError?
    do {
        try await c.action(profile)
    } catch let error as ProfileError {
        thrown = error
    }

    #expect(thrown == c.error)
    #expect(api.calls == c.calls)
    #expect(profile.me == c.me)
}

@Test func updateSendsOnlyTheChangedField() async throws {
    let api = FakeAPI(.init(getMe: [.ok(.init(body: .json(oldMe)))], updateMe: [.ok(.init(body: .json(namedMe)))]))
    let profile = await Profile(api: api)
    try await profile.load()
    try await profile.update(.init(displayName: "Ola"))

    #expect(api.updates == [.init(displayName: "Ola")])
}

struct NameCase: Sendable, CustomTestStringConvertible {
    let name: String
    let input: String
    let result: String?

    var testDescription: String { name }
}

@Test(arguments: [
    NameCase(name: "the name is trimmed", input: "  Ola  ", result: "Ola"),
    NameCase(name: "40 letters with diacritics pass", input: String(repeating: "ż", count: 40), result: String(repeating: "ż", count: 40)),
    NameCase(name: "41 letters are rejected", input: String(repeating: "ż", count: 41), result: nil),
    NameCase(name: "a blank name is rejected", input: "   ", result: nil),
    NameCase(name: "a combining accent counts as a second scalar", input: String(repeating: "e\u{0301}", count: 40), result: nil),
])
func nameRule(_ c: NameCase) {
    #expect(trimmedName(c.input) == c.result)
}

struct ErrorMessageCase: Sendable, CustomTestStringConvertible {
    let name: String
    let error: any Error
    let message: String

    var testDescription: String { name }
}

@Test(arguments: [
    ErrorMessageCase(name: "a rejected name", error: ProfileError.rejected("displayName must have 1 to 40 characters"), message: "Nazwa musi mieć od 1 do 40 znaków."),
    ErrorMessageCase(name: "a failed save", error: ProfileError.unexpected, message: "Nie udało się zapisać zmian."),
    ErrorMessageCase(name: "a rejected group name", error: GroupError.badName, message: "Nazwa musi mieć od 1 do 40 znaków."),
    ErrorMessageCase(name: "a failed group call", error: GroupError.unexpected, message: "Coś poszło nie tak. Spróbuj ponownie."),
    ErrorMessageCase(name: "an address that is not valid", error: GroupError.badEmail, message: "Nieprawidłowy adres e-mail."),
    ErrorMessageCase(name: "the invitation limit", error: GroupError.tooManyInvitations, message: "Możesz wysłać najwyżej 20 zaproszeń dziennie."),
    ErrorMessageCase(name: "an email that was not sent", error: GroupError.emailNotSent, message: "Nie udało się wysłać e-maila. Spróbuj ponownie."),
    ErrorMessageCase(name: "a dead invitation", error: InvitationError.notFound, message: "To zaproszenie jest już nieważne."),
    ErrorMessageCase(name: "too many tries for an invitation", error: InvitationError.tooManyTries, message: "Zbyt wiele prób. Spróbuj ponownie później."),
    ErrorMessageCase(name: "a rejected name of a new group", error: InvitationError.badName, message: "Nazwa musi mieć od 1 do 40 znaków."),
    ErrorMessageCase(
        name: "no network through the API client",
        error: ClientError(operationID: "updateMe", operationInput: "", causeDescription: "transport", underlyingError: URLError(.notConnectedToInternet)),
        message: "Brak połączenia z internetem."
    ),
])
func profileErrorMessage(_ c: ErrorMessageCase) {
    #expect(errorMessage(c.error) == c.message)
}
