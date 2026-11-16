import Foundation
import Testing

@testable import Slowo

/// The texts of card N of the live check of #147.
let napisalamDraft = Components.Schemas.CardCreate(
    pl: "napisałam", ru: "", examplePl: "Wczoraj ___ list do babci.", exampleRu: "", meaningRu: "", alternativesPl: [], alternativesRu: [],
    kind: .grammar,
    grammar: .init(
        topic: "czas", used: "napiszę", right: "napisałam", hint: "(napisać)", facts: [], slots: [],
        rule: "O tym, co było wczoraj, mówimy w czasie przeszłym: napisałam.", examples: ["Wczoraj napisałam list.", "Wczoraj byłam w kinie."]
    )
)

func checkAccepted(_ id: String = "k1") -> Operations.CreateGrammarCheck.Output {
    .accepted(.init(body: .json(.init(id: id, status: .queued))))
}

func checkRead(_ check: Components.Schemas.GrammarCheck) -> Operations.GetGrammarCheck.Output {
    .ok(.init(body: .json(check)))
}

let checkQueued = Components.Schemas.GrammarCheck(id: "k1", status: .queued)
let checkRunning = Components.Schemas.GrammarCheck(id: "k1", status: .running)
let checkCards = Components.Schemas.GrammarCheck(id: "k1", status: .done, cards: [psyDraft, napisalamDraft])
let checkClean = Components.Schemas.GrammarCheck(id: "k1", status: .done)
let checkClean2 = Components.Schemas.GrammarCheck(id: "k2", status: .done)

struct CheckCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    let action: @Sendable @MainActor (GrammarCheck) async throws -> Void
    let calls: [String]
    let inputs: [String]
    var error: GrammarCheckError?
    /// The cards of the check that the model holds, in the order of the check.
    var cards: [Components.Schemas.CardCreate] = []
    var clean = false

    var testDescription: String { name }
}

let checkCases: [CheckCase] = [
    CheckCase(
        name: "a check with two mistakes gives two cards",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [checkRead(checkQueued), checkRead(checkRunning), checkRead(checkCards)]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck", "getGrammarCheck", "getGrammarCheck"],
        inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1", "getGrammarCheck g1 k1", "getGrammarCheck g1 k1"],
        cards: [psyDraft, napisalamDraft]
    ),
    CheckCase(
        name: "a done check with no cards member found no mistake",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()], getGrammarCheck: [checkRead(checkClean)]),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        clean: true
    ),
    CheckCase(
        name: "a done check with an empty list found no mistake",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [checkRead(.init(id: "k1", status: .done, cards: []))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        clean: true
    ),
    CheckCase(
        name: "the text of a request is trimmed",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()], getGrammarCheck: [checkRead(checkClean)]),
        action: { try await $0.request(text: " dwa piesy \n") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        clean: true
    ),
    CheckCase(
        name: "a line break in the text becomes a space",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()], getGrammarCheck: [checkRead(checkClean)]),
        action: { try await $0.request(text: "dwa\npiesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        clean: true
    ),
    CheckCase(
        name: "a refused check",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [checkRead(.init(id: "k1", status: .failed, error: .refused))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        error: .refused
    ),
    CheckCase(
        name: "an invalid check",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [checkRead(.init(id: "k1", status: .failed, error: .invalid))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        error: .invalid
    ),
    CheckCase(
        name: "an unavailable model",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [checkRead(.init(id: "k1", status: .failed, error: .unavailable))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        error: .unavailable
    ),
    CheckCase(
        name: "a failed check with no cause",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [checkRead(.init(id: "k1", status: .failed, error: nil))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        error: .unavailable
    ),
    CheckCase(
        name: "a card with no grammar member",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [checkRead(.init(id: "k1", status: .done, cards: [domDraft]))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        error: .invalid
    ),
    CheckCase(
        name: "a text that the server refuses",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            createGrammarCheck: [.badRequest(.init(body: .json(.init(message: "text must have 1 to 500 characters"))))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy"],
        error: .badText
    ),
    CheckCase(
        name: "a refused request loads the profile",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(grouped)))],
            createGrammarCheck: [.forbidden(.init(body: .json(.init(message: "AI consent is required"))))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getMe"], inputs: ["createGrammarCheck g1 dwa piesy"]
    ),
    CheckCase(
        name: "a job that did not start",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            createGrammarCheck: [.badGateway(.init(body: .json(.init(message: "the grammar check did not start"))))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy"],
        error: .unavailable
    ),
    CheckCase(
        name: "a server error of the request",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            createGrammarCheck: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy"],
        error: .unavailable
    ),
    CheckCase(
        name: "a read that is not 200",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        error: .unavailable
    ),
    CheckCase(
        name: "a read of an unknown check",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [.notFound(.init(body: .json(.init(message: "grammar check not found"))))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"],
        error: .unavailable
    ),
    CheckCase(
        name: "a refused read loads the profile",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck", "getMe"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"]
    ),
    CheckCase(
        name: "a check that does not end",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: Array(repeating: checkRead(checkRunning), count: 215)
        ),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe", "createGrammarCheck"] + Array(repeating: "getGrammarCheck", count: 214),
        inputs: ["createGrammarCheck g1 dwa piesy"] + Array(repeating: "getGrammarCheck g1 k1", count: 214),
        error: .unavailable
    ),
    CheckCase(
        name: "the same text after a failed read reads the same check",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()],
            getGrammarCheck: [.internalServerError(.init(body: .json(.init(message: "internal error")))), checkRead(checkCards)]
        ),
        action: { check in
            try? await check.request(text: "dwa piesy")
            try await check.request(text: " dwa piesy ")
        },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck", "getGrammarCheck"],
        inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1", "getGrammarCheck g1 k1"],
        cards: [psyDraft, napisalamDraft]
    ),
    CheckCase(
        name: "another text after a failed read starts a new check",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted(), checkAccepted("k2")],
            getGrammarCheck: [.internalServerError(.init(body: .json(.init(message: "internal error")))), checkRead(checkClean2)]
        ),
        action: { check in
            try? await check.request(text: "dwa piesy")
            try await check.request(text: "trzy koty")
        },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck", "createGrammarCheck", "getGrammarCheck"],
        inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1", "createGrammarCheck g1 trzy koty", "getGrammarCheck g1 k2"],
        clean: true
    ),
    CheckCase(
        name: "the same text after a done check starts a new check",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted(), checkAccepted("k2")],
            getGrammarCheck: [checkRead(checkClean), checkRead(checkClean2)]
        ),
        action: { check in
            try await check.request(text: "dwa piesy")
            try await check.request(text: "dwa piesy")
        },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck", "createGrammarCheck", "getGrammarCheck"],
        inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1", "createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k2"],
        clean: true
    ),
    CheckCase(
        name: "the same text after a failed check starts a new check",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted(), checkAccepted("k2")],
            getGrammarCheck: [checkRead(.init(id: "k1", status: .failed, error: .unavailable)), checkRead(checkClean2)]
        ),
        action: { check in
            try? await check.request(text: "dwa piesy")
            try await check.request(text: "dwa piesy")
        },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck", "createGrammarCheck", "getGrammarCheck"],
        inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1", "createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k2"],
        clean: true
    ),
    CheckCase(
        name: "the same text after an unknown check starts a new check",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted(), checkAccepted("k2")],
            getGrammarCheck: [.notFound(.init(body: .json(.init(message: "grammar check not found")))), checkRead(checkClean2)]
        ),
        action: { check in
            try? await check.request(text: "dwa piesy")
            try await check.request(text: "dwa piesy")
        },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck", "createGrammarCheck", "getGrammarCheck"],
        inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1", "createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k2"],
        clean: true
    ),
    CheckCase(
        name: "reset removes the result of a check with no mistake",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()], getGrammarCheck: [checkRead(checkClean)]),
        action: { check in
            try await check.request(text: "dwa piesy")
            check.reset()
        },
        calls: ["getMe", "createGrammarCheck", "getGrammarCheck"], inputs: ["createGrammarCheck g1 dwa piesy", "getGrammarCheck g1 k1"]
    ),
    CheckCase(
        name: "without a group nothing is sent",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))]),
        action: { try await $0.request(text: "dwa piesy") },
        calls: ["getMe"], inputs: []
    ),
]

@MainActor
@Test(arguments: checkCases)
func grammarCheck(_ c: CheckCase) async throws {
    let api = FakeAPI(c.answers)
    let profile = Profile(api: api)
    try await profile.load()
    let check = GrammarCheck(api: api, profile: profile, collection: Cards(api: api, profile: profile), pause: .zero)

    var thrown: GrammarCheckError?
    do {
        try await c.action(check)
    } catch let error as GrammarCheckError {
        thrown = error
    }

    #expect(thrown == c.error)
    #expect(api.calls == c.calls)
    #expect(api.inputs == c.inputs)
    #expect(check.cards == c.cards.enumerated().compactMap { GrammarFields(id: $0.offset, $0.element) })
    #expect(check.clean == c.clean)
    #expect(!check.waiting)
}

@MainActor
@Test func aReadThatThrowsKeepsTheCheck() async throws {
    let api = FakeAPI(.init(getMe: [.ok(.init(body: .json(grouped)))], createGrammarCheck: [checkAccepted()]))
    let profile = Profile(api: api)
    try await profile.load()
    let check = GrammarCheck(api: api, profile: profile, collection: Cards(api: api, profile: profile), pause: .zero)

    await #expect(throws: FakeAPI.Empty.self) { try await check.request(text: "dwa piesy") }
    #expect(!check.waiting)
    await #expect(throws: FakeAPI.Empty.self) { try await check.request(text: "dwa piesy") }
    #expect(api.calls == ["getMe", "createGrammarCheck", "getGrammarCheck", "getGrammarCheck"])
}
