import Foundation
import Testing

@testable import Slowo

let cebula = Components.Schemas.Card(
    id: "c1", kind: .translation, pl: "Cebula", ru: "Лук", examplePl: "Kroję cebulę.", exampleRu: "Я режу лук.", meaningRu: "овощ",
    alternativesPl: [], alternativesRu: []
)
let dom = Components.Schemas.Card(
    id: "c2", kind: .translation, pl: "Dom", ru: "Дом", examplePl: "To jest dom.", exampleRu: "Это дом.", meaningRu: "здание",
    alternativesPl: ["Domek"], alternativesRu: []
)
let rule = Components.Schemas.Card(
    id: "c3", kind: .grammar, pl: "Dom", ru: "Дом", examplePl: "To jest dom.", exampleRu: "Это дом.", meaningRu: "здание",
    alternativesPl: ["Domek"], alternativesRu: []
)

struct CardsCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    let action: @Sendable @MainActor (Cards, Profile) async throws -> Void
    let calls: [String]
    let inputs: [String]
    let error: CardError?
    let cards: [Components.Schemas.Card]?
    let query: String

    var testDescription: String { name }
}

let cardsCases: [CardsCase] = [
    CardsCase(
        name: "load reads the cards of the group",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], listCards: [.ok(.init(body: .json(.init(cards: [cebula, dom]))))]),
        action: { cards, _ in try await cards.load(query: "") },
        calls: ["getMe", "listCards"], inputs: ["listCards g1 -"], error: nil, cards: [cebula, dom], query: ""
    ),
    CardsCase(
        name: "a search text goes to the server",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], listCards: [.ok(.init(body: .json(.init(cards: [dom]))))]),
        action: { cards, _ in try await cards.load(query: "dom") },
        calls: ["getMe", "listCards"], inputs: ["listCards g1 dom"], error: nil, cards: [dom], query: "dom"
    ),
    CardsCase(
        name: "a search text is trimmed",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], listCards: [.ok(.init(body: .json(.init(cards: [dom]))))]),
        action: { cards, _ in try await cards.load(query: " do m \n") },
        calls: ["getMe", "listCards"], inputs: ["listCards g1 do m"], error: nil, cards: [dom], query: "do m"
    ),
    CardsCase(
        name: "the list keeps the order of the answer",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], listCards: [.ok(.init(body: .json(.init(cards: [dom, cebula]))))]),
        action: { cards, _ in try await cards.load(query: "d") },
        calls: ["getMe", "listCards"], inputs: ["listCards g1 d"], error: nil, cards: [dom, cebula], query: "d"
    ),
    CardsCase(
        name: "a text of spaces is no search",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], listCards: [.ok(.init(body: .json(.init(cards: [cebula]))))]),
        action: { cards, _ in try await cards.load(query: " \n ") },
        calls: ["getMe", "listCards"], inputs: ["listCards g1 -"], error: nil, cards: [cebula], query: ""
    ),
    CardsCase(
        name: "a grammar card is not in the list",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], listCards: [.ok(.init(body: .json(.init(cards: [cebula, rule, dom]))))]),
        action: { cards, _ in try await cards.load(query: "") },
        calls: ["getMe", "listCards"], inputs: ["listCards g1 -"], error: nil, cards: [cebula, dom], query: ""
    ),
    CardsCase(
        name: "a group with no cards has an empty list",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], listCards: [.ok(.init(body: .json(.init(cards: []))))]),
        action: { cards, _ in try await cards.load(query: "") },
        calls: ["getMe", "listCards"], inputs: ["listCards g1 -"], error: nil, cards: [], query: ""
    ),
    CardsCase(
        name: "without a group nothing loads",
        answers: .init(getMe: [.ok(.init(body: .json(alone)))]),
        action: { cards, _ in try await cards.load(query: "") },
        calls: ["getMe"], inputs: [], error: nil, cards: nil, query: ""
    ),
    CardsCase(
        name: "a removed member has no list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            listCards: [
                .ok(.init(body: .json(.init(cards: [cebula])))),
                .forbidden(.init(body: .json(.init(message: "not a member of this group")))),
            ]
        ),
        action: { cards, _ in
            try await cards.load(query: "ceb")
            try await cards.load(query: "ceb")
        },
        calls: ["getMe", "listCards", "listCards", "getMe"], inputs: ["listCards g1 ceb", "listCards g1 ceb"], error: nil, cards: nil, query: ""
    ),
    CardsCase(
        name: "a failed load has no list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { cards, _ in try await cards.load(query: "") },
        calls: ["getMe", "listCards"], inputs: ["listCards g1 -"], error: .unexpected, cards: nil, query: ""
    ),
    CardsCase(
        name: "a failed reload keeps the list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [
                .ok(.init(body: .json(.init(cards: [cebula])))),
                .internalServerError(.init(body: .json(.init(message: "internal error")))),
            ]
        ),
        action: { cards, _ in
            try await cards.load(query: "")
            try await cards.load(query: "")
        },
        calls: ["getMe", "listCards", "listCards"], inputs: ["listCards g1 -", "listCards g1 -"], error: .unexpected, cards: [cebula], query: ""
    ),
    CardsCase(
        name: "a refused search text keeps the list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [
                .ok(.init(body: .json(.init(cards: [cebula])))),
                .badRequest(.init(body: .json(.init(message: "q must have at most 200 characters")))),
            ]
        ),
        action: { cards, _ in
            try await cards.load(query: "ceb")
            try await cards.load(query: "x")
        },
        calls: ["getMe", "listCards", "listCards"], inputs: ["listCards g1 ceb", "listCards g1 x"], error: .unexpected, cards: [cebula], query: "ceb"
    ),
    CardsCase(
        name: "a move to another group clears the old list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(inG2)))],
            listCards: [
                .ok(.init(body: .json(.init(cards: [cebula])))),
                .internalServerError(.init(body: .json(.init(message: "internal error")))),
            ]
        ),
        action: { cards, profile in
            try await cards.load(query: "ceb")
            try await profile.load()
            try await cards.load(query: "ceb")
        },
        calls: ["getMe", "listCards", "getMe", "listCards"], inputs: ["listCards g1 ceb", "listCards g2 ceb"], error: .unexpected, cards: nil, query: ""
    ),
    CardsCase(
        name: "an edit loads the list again with its search text",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula, dom])))), .ok(.init(body: .json(.init(cards: [dom]))))],
            updateCard: [.ok(.init(body: .json(dom)))]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.update("c1", .init(pl: "Cebulka"))
        },
        calls: ["getMe", "listCards", "updateCard", "listCards"], inputs: ["listCards g1 o", "updateCard g1 c1 Cebulka", "listCards g1 o"], error: nil, cards: [dom], query: "o"
    ),
    CardsCase(
        name: "a refused edit keeps the list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula]))))],
            updateCard: [.badRequest(.init(body: .json(.init(message: "pl must have 1 to 200 characters"))))]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.update("c1", .init(pl: "x"))
        },
        calls: ["getMe", "listCards", "updateCard"], inputs: ["listCards g1 o", "updateCard g1 c1 x"], error: .rejected, cards: [cebula], query: "o"
    ),
    CardsCase(
        name: "an edit of an archived card loads the list again",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula, dom])))), .ok(.init(body: .json(.init(cards: [dom]))))],
            updateCard: [.notFound(.init(body: .json(.init(message: "card not found"))))]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.update("c1", .init(pl: "x"))
        },
        calls: ["getMe", "listCards", "updateCard", "listCards"], inputs: ["listCards g1 o", "updateCard g1 c1 x", "listCards g1 o"], error: .gone, cards: [dom], query: "o"
    ),
    CardsCase(
        name: "an edit after a removal from the group shows no list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula]))))],
            updateCard: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.update("c1", .init(pl: "x"))
        },
        calls: ["getMe", "listCards", "updateCard", "getMe"], inputs: ["listCards g1 o", "updateCard g1 c1 x"], error: nil, cards: nil, query: ""
    ),
    CardsCase(
        name: "a failed edit keeps the list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula]))))],
            updateCard: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.update("c1", .init(pl: "x"))
        },
        calls: ["getMe", "listCards", "updateCard"], inputs: ["listCards g1 o", "updateCard g1 c1 x"], error: .unexpected, cards: [cebula], query: "o"
    ),
    CardsCase(
        name: "an archive loads the list again",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula, dom])))), .ok(.init(body: .json(.init(cards: [dom]))))],
            archiveCard: [.noContent(.init())]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.archive("c1")
        },
        calls: ["getMe", "listCards", "archiveCard", "listCards"], inputs: ["listCards g1 o", "archiveCard g1 c1", "listCards g1 o"], error: nil, cards: [dom], query: "o"
    ),
    CardsCase(
        name: "an archive of an archived card is no error",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula, dom])))), .ok(.init(body: .json(.init(cards: [dom]))))],
            archiveCard: [.notFound(.init(body: .json(.init(message: "card not found"))))]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.archive("c1")
        },
        calls: ["getMe", "listCards", "archiveCard", "listCards"], inputs: ["listCards g1 o", "archiveCard g1 c1", "listCards g1 o"], error: nil, cards: [dom], query: "o"
    ),
    CardsCase(
        name: "an archive after a removal from the group shows no list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula]))))],
            archiveCard: [.forbidden(.init(body: .json(.init(message: "not a member of this group"))))]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.archive("c1")
        },
        calls: ["getMe", "listCards", "archiveCard", "getMe"], inputs: ["listCards g1 o", "archiveCard g1 c1"], error: nil, cards: nil, query: ""
    ),
    CardsCase(
        name: "a failed archive keeps the list",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula, dom]))))],
            archiveCard: [.internalServerError(.init(body: .json(.init(message: "internal error"))))]
        ),
        action: { cards, _ in
            try await cards.load(query: "o")
            try await cards.archive("c1")
        },
        calls: ["getMe", "listCards", "archiveCard"], inputs: ["listCards g1 o", "archiveCard g1 c1"], error: .unexpected, cards: [cebula, dom], query: "o"
    ),
]

@MainActor
@Test(arguments: cardsCases)
func cards(_ c: CardsCase) async throws {
    let api = FakeAPI(c.answers)
    let profile = Profile(api: api)
    try await profile.load()
    let cards = Cards(api: api, profile: profile)

    var thrown: CardError?
    do {
        try await c.action(cards, profile)
    } catch let error as CardError {
        thrown = error
    }

    #expect(thrown == c.error)
    #expect(api.calls == c.calls)
    #expect(api.inputs == c.inputs)
    #expect(cards.cards == c.cards)
    #expect(cards.query == c.query)
}

struct DroppedAnswerCase: Sendable, CustomTestStringConvertible {
    let name: String
    let first: String
    let between: @Sendable @MainActor (Profile) async throws -> Void
    let second: String
    let inputs: [String]

    var testDescription: String { name }
}

@MainActor
@Test(.timeLimit(.minutes(1)), arguments: [
    DroppedAnswerCase(name: "another search text", first: "c", between: { _ in }, second: "do", inputs: ["listCards g1 c", "listCards g1 do"]),
    DroppedAnswerCase(name: "another group", first: "", between: { try await $0.load() }, second: "", inputs: ["listCards g1 -", "listCards g2 -"]),
])
func anOlderAnswerIsDropped(_ c: DroppedAnswerCase) async throws {
    let (signal, release) = AsyncStream<Void>.makeStream()
    let api = FakeAPI(
        .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(inG2)))],
            listCards: [.ok(.init(body: .json(.init(cards: [cebula])))), .ok(.init(body: .json(.init(cards: [dom]))))]
        ),
        holdListCards: { call in
            guard call == 1 else { return }
            for await _ in signal {}
        }
    )
    let profile = Profile(api: api)
    try await profile.load()
    let cards = Cards(api: api, profile: profile)

    let first = Task { try await cards.load(query: c.first) }
    while !api.calls.contains("listCards") {
        await Task.yield()
    }
    try await c.between(profile)
    try await cards.load(query: c.second)
    release.finish()
    try await first.value

    #expect(api.inputs == c.inputs)
    #expect(cards.cards == [dom])
}
