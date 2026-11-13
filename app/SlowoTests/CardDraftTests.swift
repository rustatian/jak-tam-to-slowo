import Foundation
import Testing

@testable import Slowo

let queued = Components.Schemas.Draft(id: "d1", status: .queued)
let running = Components.Schemas.Draft(id: "d1", status: .running)
let doneCard = Components.Schemas.Draft(id: "d1", status: .done, card: domDraft)
let doneChoices = Components.Schemas.Draft(id: "d1", status: .done, choices: ["овощ", "оружие"])

func accepted(_ draft: Components.Schemas.Draft) -> Operations.CreateDraft.Output {
    .accepted(.init(body: .json(draft)))
}

func read(_ draft: Components.Schemas.Draft) -> Operations.GetDraft.Output {
    .ok(.init(body: .json(draft)))
}

struct DraftCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    let action: @Sendable @MainActor (CardDraft) async throws -> Void
    let calls: [String]
    let inputs: [String]
    let error: DraftError?
    let step: CardDraft.Step

    var testDescription: String { name }
}

let draftCases: [DraftCase] = [
    DraftCase(
        name: "a draft with one meaning becomes a card",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)], getDraft: [read(queued), read(running), read(doneCard)]),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft", "getDraft", "getDraft"],
        inputs: ["createDraft g1 лук -", "getDraft g1 d1", "getDraft g1 d1", "getDraft g1 d1"],
        error: nil, step: .card(CardFields(domDraft))
    ),
    DraftCase(
        name: "the text of a request is trimmed",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)], getDraft: [read(doneCard)]),
        action: { try await $0.request(text: " лук \n") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: nil, step: .card(CardFields(domDraft))
    ),
    DraftCase(
        name: "a word with more meanings gives the meanings",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)], getDraft: [read(doneChoices)]),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: nil, step: .choices(["овощ", "оружие"])
    ),
    DraftCase(
        name: "a request with a meaning sends the text and the meaning",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued), accepted(queued)], getDraft: [read(doneChoices), read(doneCard)]
        ),
        action: { draft in
            try await draft.request(text: " лук ")
            try await draft.request(text: " лук ", meaning: "оружие")
        },
        calls: ["getMe", "createDraft", "getDraft", "createDraft", "getDraft"],
        inputs: ["createDraft g1 лук -", "getDraft g1 d1", "createDraft g1 лук оружие", "getDraft g1 d1"],
        error: nil, step: .card(CardFields(domDraft))
    ),
    DraftCase(
        name: "a failed request with a meaning keeps the meanings",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            createDraft: [accepted(queued), .badGateway(.init(body: .json(.init(message: "the draft job did not start"))))],
            getDraft: [read(doneChoices)]
        ),
        action: { draft in
            try await draft.request(text: "лук")
            try await draft.request(text: "лук", meaning: "оружие")
        },
        calls: ["getMe", "createDraft", "getDraft", "createDraft"],
        inputs: ["createDraft g1 лук -", "getDraft g1 d1", "createDraft g1 лук оружие"],
        error: .unavailable, step: .choices(["овощ", "оружие"])
    ),
    DraftCase(
        name: "a refused draft",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)],
            getDraft: [read(.init(id: "d1", status: .failed, error: .refused))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: .refused, step: .idle
    ),
    DraftCase(
        name: "an invalid draft",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)],
            getDraft: [read(.init(id: "d1", status: .failed, error: .invalid))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: .invalid, step: .idle
    ),
    DraftCase(
        name: "an unavailable model",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)],
            getDraft: [read(.init(id: "d1", status: .failed, error: .unavailable))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: .unavailable, step: .idle
    ),
    DraftCase(
        name: "a failed draft with no cause",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)], getDraft: [read(.init(id: "d1", status: .failed))]),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: .unavailable, step: .idle
    ),
    DraftCase(
        name: "a done draft with no card and no meanings",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)], getDraft: [read(.init(id: "d1", status: .done))]),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: .invalid, step: .idle
    ),
    DraftCase(
        name: "a text that the server refuses",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            createDraft: [.badRequest(.init(body: .json(.init(message: "text must have 1 to 200 characters"))))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft"], inputs: ["createDraft g1 лук -"], error: .badText, step: .idle
    ),
    DraftCase(
        name: "a refused request loads the profile",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(grouped)))],
            createDraft: [.forbidden(.init(body: .json(.init(message: "AI consent is required"))))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getMe"], inputs: ["createDraft g1 лук -"], error: nil, step: .idle
    ),
    DraftCase(
        name: "a job that did not start",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            createDraft: [.badGateway(.init(body: .json(.init(message: "the draft job did not start"))))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft"], inputs: ["createDraft g1 лук -"], error: .unavailable, step: .idle
    ),
    DraftCase(
        name: "a server error of the request",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            createDraft: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft"], inputs: ["createDraft g1 лук -"], error: .unavailable, step: .idle
    ),
    DraftCase(
        name: "a draft that the server does not have",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)],
            getDraft: [.notFound(.init(body: .json(.init(message: "draft not found"))))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: .unavailable, step: .idle
    ),
    DraftCase(
        name: "a server error of a read",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)],
            getDraft: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft", "getDraft"], inputs: ["createDraft g1 лук -", "getDraft g1 d1"], error: .unavailable, step: .idle
    ),
    DraftCase(
        name: "a draft that does not end stops after 80 reads",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))], createDraft: [accepted(queued)], getDraft: Array(repeating: read(running), count: 81)
        ),
        action: { try await $0.request(text: "лук") },
        calls: ["getMe", "createDraft"] + Array(repeating: "getDraft", count: 80),
        inputs: ["createDraft g1 лук -"] + Array(repeating: "getDraft g1 d1", count: 80),
        error: .unavailable, step: .idle
    ),
]

@MainActor
@Test(arguments: draftCases)
func cardDraft(_ c: DraftCase) async throws {
    let api = FakeAPI(c.answers)
    let profile = Profile(api: api)
    try await profile.load()
    let draft = CardDraft(api: api, profile: profile, pause: .zero)

    var thrown: DraftError?
    do {
        try await c.action(draft)
    } catch let error as DraftError {
        thrown = error
    }

    #expect(thrown == c.error)
    #expect(api.calls == c.calls)
    #expect(api.inputs == c.inputs)
    #expect(draft.step == c.step)
}

@MainActor
@Test func aCallThatThrowsEndsTheWait() async throws {
    let api = FakeAPI(.init(getMe: [.ok(.init(body: .json(grouped)))]))
    let profile = Profile(api: api)
    try await profile.load()
    let draft = CardDraft(api: api, profile: profile, pause: .zero)

    await #expect(throws: FakeAPI.Empty.self) {
        try await draft.request(text: "лук")
    }
    #expect(draft.step == .idle)
}
