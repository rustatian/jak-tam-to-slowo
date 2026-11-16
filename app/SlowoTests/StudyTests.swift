import Foundation
import Testing
import UserNotifications

@testable import Slowo

let cebula0 = Components.Schemas.StudyCard(card: cebula, progressVersion: 0)
let dom3 = Components.Schemas.StudyCard(card: dom, progressVersion: 3)
let psy0 = Components.Schemas.StudyCard(card: psy, progressVersion: 0)
let luk0 = Components.Schemas.StudyCard(card: luk, progressVersion: 0)
let later = start.addingTimeInterval(600)
let groupedPlRu = Components.Schemas.Me(id: "s1", email: "ola@example.com", displayName: "", direction: .plRu, reminder: false, groupId: "g1")

func q(_ cards: [Components.Schemas.StudyCard], next: Date? = nil) -> Operations.GetStudyQueue.Output {
    .ok(.init(body: .json(.init(cards: cards, nextDueAt: next))))
}

func graded(_ grade: Components.Schemas.Grade, _ correction: String) -> Components.Schemas.Review {
    .init(grade: grade, correction: correction)
}

struct StudyCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    let action: @Sendable @MainActor (Study, Profile) async throws -> Void
    /// The calls after the `getMe` call of the first profile load.
    let calls: [String]
    let inputs: [String]
    let error: StudyError?
    let queue: [Components.Schemas.StudyCard]?
    let nextDueAt: Date?
    let card: Components.Schemas.StudyCard?
    let review: Components.Schemas.Review?

    var testDescription: String { name }
}

let studyCases: [StudyCase] = [
    StudyCase(
        name: "load reads the queue of the group",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], getStudyQueue: [q([cebula0, dom3], next: later)]),
        action: { study, _ in try await study.load() },
        calls: ["getStudyQueue"], inputs: ["getStudyQueue g1"], error: nil, queue: [cebula0, dom3], nextDueAt: later, card: cebula0, review: nil
    ),
    StudyCase(
        name: "an empty queue keeps the next due time",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], getStudyQueue: [q([], next: later)]),
        action: { study, _ in try await study.load() },
        calls: ["getStudyQueue"], inputs: ["getStudyQueue g1"], error: nil, queue: [], nextDueAt: later, card: nil, review: nil
    ),
    StudyCase(
        name: "a grammar card is in the queue",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], getStudyQueue: [q([psy0, cebula0])]),
        action: { study, _ in try await study.load() },
        calls: ["getStudyQueue"], inputs: ["getStudyQueue g1"], error: nil, queue: [psy0, cebula0], nextDueAt: nil, card: psy0, review: nil
    ),
    StudyCase(
        name: "a reload keeps the card on screen",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], getStudyQueue: [q([cebula0, dom3]), q([dom3, luk0], next: later)]),
        action: { study, _ in
            try await study.load()
            try await study.load()
        },
        calls: ["getStudyQueue", "getStudyQueue"], inputs: ["getStudyQueue g1", "getStudyQueue g1"],
        error: nil, queue: [dom3, luk0], nextDueAt: later, card: cebula0, review: nil
    ),
    StudyCase(
        name: "a failed reload keeps the queue",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([cebula0], next: later), .internalServerError(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.load()
        },
        calls: ["getStudyQueue", "getStudyQueue"], inputs: ["getStudyQueue g1", "getStudyQueue g1"],
        error: .unexpected, queue: [cebula0], nextDueAt: later, card: cebula0, review: nil
    ),
    StudyCase(
        name: "a refused load loads the profile and clears the state",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getStudyQueue: [q([cebula0], next: later), .forbidden(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.load()
        },
        calls: ["getStudyQueue", "getStudyQueue", "getMe"], inputs: ["getStudyQueue g1", "getStudyQueue g1"],
        error: nil, queue: nil, nextDueAt: nil, card: nil, review: nil
    ),
    StudyCase(
        name: "a move to another group clears the state first",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(inG2)))],
            getStudyQueue: [q([cebula0], next: later), .internalServerError(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, profile in
            try await study.load()
            try await profile.load()
            try await study.load()
        },
        calls: ["getStudyQueue", "getMe", "getStudyQueue"], inputs: ["getStudyQueue g1", "getStudyQueue g2"],
        error: .unexpected, queue: nil, nextDueAt: nil, card: nil, review: nil
    ),
    StudyCase(
        name: "a learner without a group sees no queue",
        answers: .init(getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))], getStudyQueue: [q([cebula0])]),
        action: { study, profile in
            try await study.load()
            try await profile.load()
            try await study.load()
        },
        calls: ["getStudyQueue", "getMe"], inputs: ["getStudyQueue g1"], error: nil, queue: nil, nextDueAt: nil, card: nil, review: nil
    ),
    StudyCase(
        name: "a correct answer shows the feedback and loads the queue again",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0], next: later)],
            createReview: [.created(.init(body: .json(graded(.correct, "Dom"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer(" dom ")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: nil, queue: [luk0], nextDueAt: later, card: dom3, review: graded(.correct, "Dom")
    ),
    StudyCase(
        name: "show answer sends an empty answer",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0])],
            createReview: [.created(.init(body: .json(graded(.again, "Dom"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl -", "getStudyQueue g1"],
        error: nil, queue: [luk0], nextDueAt: nil, card: dom3, review: graded(.again, "Dom")
    ),
    StudyCase(
        name: "a repeat of a counted answer shows the stored grade",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0])],
            createReview: [.ok(.init(body: .json(graded(.almost, "Dom"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: nil, queue: [luk0], nextDueAt: nil, card: dom3, review: graded(.almost, "Dom")
    ),
    StudyCase(
        name: "the answer goes in the direction of the profile",
        answers: .init(
            getMe: [.ok(.init(body: .json(groupedPlRu)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0])],
            createReview: [.created(.init(body: .json(graded(.correct, "Дом"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("дом")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 pl-ru дом", "getStudyQueue g1"],
        error: nil, queue: [luk0], nextDueAt: nil, card: dom3, review: graded(.correct, "Дом")
    ),
    StudyCase(
        name: "a failed reload after an answer keeps the old queue",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), .internalServerError(.init(body: .json(.init(message: "x"))))],
            createReview: [.created(.init(body: .json(graded(.correct, "Dom"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: nil, queue: [dom3, luk0], nextDueAt: nil, card: dom3, review: graded(.correct, "Dom")
    ),
    StudyCase(
        name: "next shows the next card",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0])],
            createReview: [.created(.init(body: .json(graded(.correct, "Dom"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
            study.next()
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: nil, queue: [luk0], nextDueAt: nil, card: luk0, review: nil
    ),
    StudyCase(
        name: "next after a failed reload skips the answered card",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), .internalServerError(.init(body: .json(.init(message: "x"))))],
            createReview: [.created(.init(body: .json(graded(.correct, "Dom"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
            study.next()
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: nil, queue: [luk0], nextDueAt: nil, card: luk0, review: nil
    ),
    StudyCase(
        name: "next after the last card shows the empty state",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3]), q([], next: later)],
            createReview: [.created(.init(body: .json(graded(.correct, "Dom"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
            study.next()
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: nil, queue: [], nextDueAt: later, card: nil, review: nil
    ),
    StudyCase(
        name: "a stale progress version shows the next card",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([cebula0, luk0])],
            createReview: [.conflict(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: .changed, queue: [cebula0, luk0], nextDueAt: nil, card: cebula0, review: nil
    ),
    StudyCase(
        name: "an archived card shows the next card",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([cebula0, luk0])],
            createReview: [.notFound(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: .changed, queue: [cebula0, luk0], nextDueAt: nil, card: cebula0, review: nil
    ),
    StudyCase(
        name: "a conflict with a failed reload shows the next card",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), .internalServerError(.init(body: .json(.init(message: "x"))))],
            createReview: [.conflict(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: .changed, queue: [luk0], nextDueAt: nil, card: luk0, review: nil
    ),
    StudyCase(
        name: "a conflict on the last card shows a card of the new queue",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3]), q([cebula0], next: later)],
            createReview: [.conflict(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview", "getStudyQueue"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom", "getStudyQueue g1"],
        error: .changed, queue: [cebula0], nextDueAt: later, card: cebula0, review: nil
    ),
    StudyCase(
        name: "an unavailable AI model keeps the card",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0])],
            createReview: [.serviceUnavailable(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom"],
        error: .aiUnavailable, queue: [dom3, luk0], nextDueAt: nil, card: dom3, review: nil
    ),
    StudyCase(
        name: "a refused answer is an unexpected error",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0])],
            createReview: [.badRequest(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom"],
        error: .unexpected, queue: [dom3, luk0], nextDueAt: nil, card: dom3, review: nil
    ),
    StudyCase(
        name: "a server error of a review keeps the card",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0])],
            createReview: [.internalServerError(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom"],
        error: .unexpected, queue: [dom3, luk0], nextDueAt: nil, card: dom3, review: nil
    ),
    StudyCase(
        name: "a refused review loads the profile and clears the state",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped))), .ok(.init(body: .json(alone)))],
            getStudyQueue: [q([dom3, luk0])],
            createReview: [.forbidden(.init(body: .json(.init(message: "x"))))]
        ),
        action: { study, _ in
            try await study.load()
            try await study.answer("dom")
        },
        calls: ["getStudyQueue", "createReview", "getMe"], inputs: ["getStudyQueue g1", "createReview g1 c2 3 ru-pl dom"],
        error: nil, queue: nil, nextDueAt: nil, card: nil, review: nil
    ),
]

@MainActor
@Test(arguments: studyCases)
func study(_ c: StudyCase) async throws {
    let api = FakeAPI(c.answers)
    let profile = Profile(api: api)
    try await profile.load()
    let study = Study(api: api, profile: profile, reminder: Reminder(center: FakeNotifications(status: .notDetermined), profile: profile))

    var thrown: StudyError?
    do {
        try await c.action(study, profile)
    } catch let error as StudyError {
        thrown = error
    }

    #expect(thrown == c.error)
    #expect(api.calls == ["getMe"] + c.calls)
    #expect(api.inputs == c.inputs)
    #expect(study.queue == c.queue)
    #expect(study.nextDueAt == c.nextDueAt)
    #expect(study.card == c.card)
    #expect(study.review == c.review)
}

struct AttemptCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answers: FakeAPI.Answers
    /// The steps after the first `load()`. Each step ignores its error.
    let action: @Sendable @MainActor (Study, Profile) async -> Void
    /// Whether the two reviews have the same attempt ID.
    let same: Bool

    var testDescription: String { name }
}

let attemptCases: [AttemptCase] = [
    AttemptCase(
        name: "a retry after a network error sends the same attempt",
        answers: .init(getMe: [.ok(.init(body: .json(grouped)))], getStudyQueue: [q([dom3, luk0])]),
        action: { study, _ in
            try? await study.answer("dom")
            try? await study.answer("dom")
        },
        same: true
    ),
    AttemptCase(
        name: "a retry after an unavailable AI model sends the same attempt",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0])],
            createReview: [.serviceUnavailable(.init(body: .json(.init(message: "x")))), .created(.init(body: .json(graded(.correct, "Dom"))))]
        ),
        action: { study, _ in
            try? await study.answer("dom")
            try? await study.answer("dom")
        },
        same: true
    ),
    AttemptCase(
        name: "a retry after a server error sends the same attempt",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0])],
            createReview: [.internalServerError(.init(body: .json(.init(message: "x")))), .created(.init(body: .json(graded(.correct, "Dom"))))]
        ),
        action: { study, _ in
            try? await study.answer("dom")
            try? await study.answer("dom")
        },
        same: true
    ),
    AttemptCase(
        name: "a changed answer is a new attempt",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0])],
            createReview: [.serviceUnavailable(.init(body: .json(.init(message: "x")))), .created(.init(body: .json(graded(.correct, "Dom"))))]
        ),
        action: { study, _ in
            try? await study.answer("dom")
            try? await study.answer("domek")
        },
        same: false
    ),
    AttemptCase(
        name: "a new direction is a new attempt",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            updateMe: [.ok(.init(body: .json(groupedPlRu)))],
            getStudyQueue: [q([dom3, luk0]), q([luk0])],
            createReview: [.serviceUnavailable(.init(body: .json(.init(message: "x")))), .created(.init(body: .json(graded(.correct, "Дом"))))]
        ),
        action: { study, profile in
            try? await study.answer("dom")
            try? await profile.update(.init(direction: .plRu))
            try? await study.answer("dom")
        },
        same: false
    ),
    AttemptCase(
        name: "the next card is a new attempt",
        answers: .init(
            getMe: [.ok(.init(body: .json(grouped)))],
            getStudyQueue: [q([cebula0, luk0]), q([luk0]), q([])],
            createReview: [.created(.init(body: .json(graded(.again, "Cebula")))), .created(.init(body: .json(graded(.again, "Łuk"))))]
        ),
        action: { study, _ in
            try? await study.answer("")
            study.next()
            try? await study.answer("")
        },
        same: false
    ),
]

@MainActor
@Test(arguments: attemptCases)
func attempt(_ c: AttemptCase) async throws {
    let api = FakeAPI(c.answers)
    let profile = Profile(api: api)
    try await profile.load()
    let study = Study(api: api, profile: profile, reminder: Reminder(center: FakeNotifications(status: .notDetermined), profile: profile))
    try await study.load()
    await c.action(study, profile)

    let reviews = api.reviews
    try #require(reviews.count == 2)
    #expect(UUID(uuidString: reviews[0].attemptId) != nil)
    #expect(UUID(uuidString: reviews[1].attemptId) != nil)
    #expect((reviews[0].attemptId == reviews[1].attemptId) == c.same)
}

struct StudyReminderCase: Sendable, CustomTestStringConvertible {
    let name: String
    let me: Components.Schemas.Me
    let getStudyQueue: [Operations.GetStudyQueue.Output]
    let calls: [String]

    var testDescription: String { name }
}

let studyReminderCases: [StudyReminderCase] = [
    StudyReminderCase(
        name: "a queue answer goes to the reminder",
        me: reminded, getStudyQueue: [q([cebula0], next: later)], calls: [scheduled]
    ),
    StudyReminderCase(
        name: "a failed load keeps the notification",
        me: reminded, getStudyQueue: [.internalServerError(.init(body: .json(.init(message: "x"))))], calls: []
    ),
    StudyReminderCase(
        name: "a load without a group removes the notification",
        me: alone, getStudyQueue: [], calls: [removed]
    ),
]

@MainActor
@Test(arguments: studyReminderCases)
func studyReminder(_ c: StudyReminderCase) async throws {
    let api = FakeAPI(.init(getMe: [.ok(.init(body: .json(c.me)))], getStudyQueue: c.getStudyQueue))
    let profile = Profile(api: api)
    try await profile.load()
    let center = FakeNotifications(status: .authorized)
    let study = Study(api: api, profile: profile, reminder: Reminder(center: center, profile: profile, now: { start }))
    try? await study.load()

    #expect(center.calls == c.calls)
}
