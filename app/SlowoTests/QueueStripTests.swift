import Foundation
import Testing

@testable import Slowo

/// A study card with the ID and the counts of the learner.
private func item(_ id: String, correct: Int = 0, incorrect: Int = 0) -> Components.Schemas.StudyCard {
    var card = dom
    card.id = id
    return .init(card: card, progressVersion: 0, correct: correct, incorrect: incorrect)
}

/// The new cards c1 to c11.
private let cards = (1...11).map { item("c\($0)") }
private let c1 = cards[0]
private let c2 = cards[1]
private let c3 = cards[2]
private let h = item("h", correct: 0, incorrect: 1)
private let e = item("e", correct: 1, incorrect: 1)
private let g = item("g", correct: 2, incorrect: 1)

struct StripCase: Sendable, CustomTestStringConvertible {
    let name: String
    let card: Components.Schemas.StudyCard
    let grade: Components.Schemas.Grade?
    let queue: [Components.Schemas.StudyCard]
    let due: Int
    let segments: [QueueStrip.Segment]
    let more: Int

    var testDescription: String { name }
}

let stripCases: [StripCase] = [
    StripCase(
        name: "before the grade the first segment is the card on screen", card: c1, grade: nil, queue: [c1, c2, c3], due: 3,
        segments: [.current, .waiting, .waiting], more: 0
    ),
    StripCase(name: "a correct grade", card: c1, grade: .correct, queue: [c2, c3], due: 2, segments: [.good, .waiting, .waiting], more: 0),
    StripCase(name: "an almost grade", card: c1, grade: .almost, queue: [c2, c3], due: 2, segments: [.almost, .waiting, .waiting], more: 0),
    StripCase(name: "a grade of again", card: c1, grade: .again, queue: [c2, c3], due: 2, segments: [.miss, .waiting, .waiting], more: 0),
    StripCase(
        name: "only a card with more misses than correct answers is heavy", card: c1, grade: nil, queue: [c1, h, e, g], due: 4,
        segments: [.current, .heavy, .waiting, .waiting], more: 0
    ),
    StripCase(
        name: "more than 10 due cards before the grade", card: c1, grade: nil, queue: Array(cards.prefix(10)), due: 12,
        segments: [.current, .waiting, .waiting, .waiting, .waiting, .waiting, .waiting, .waiting, .waiting, .waiting], more: 2
    ),
    StripCase(
        name: "more than 10 due cards after the grade", card: c1, grade: .correct, queue: Array(cards.dropFirst()), due: 11,
        segments: [.good, .waiting, .waiting, .waiting, .waiting, .waiting, .waiting, .waiting, .waiting, .waiting], more: 2
    ),
    StripCase(
        name: "the answered card is still in the queue after a failed load", card: c1, grade: .again, queue: [c1, c2, c3], due: 3,
        segments: [.miss, .waiting, .waiting], more: 0
    ),
    StripCase(
        name: "the queue does not hold the card on screen before the grade", card: c1, grade: nil, queue: [c2, c3], due: 2,
        segments: [.current, .waiting, .waiting], more: 0
    ),
    StripCase(name: "the card on screen is the only due card", card: c1, grade: nil, queue: [c1], due: 1, segments: [.current], more: 0),
]

@Test(arguments: stripCases)
func queueStrip(_ c: StripCase) {
    let strip = QueueStrip(card: c.card, grade: c.grade, queue: c.queue, due: c.due)
    #expect(strip.segments == c.segments)
    #expect(strip.more == c.more)
}

struct WeightCase: Sendable, CustomTestStringConvertible {
    let name: String
    let correct: Int
    let incorrect: Int
    let text: String

    var testDescription: String { name }
}

let weightCases: [WeightCase] = [
    WeightCase(name: "a new card", correct: 0, incorrect: 0, text: "waga 2,0"),
    WeightCase(name: "one correct answer", correct: 1, incorrect: 0, text: "waga 1,0"),
    WeightCase(name: "two correct answers", correct: 2, incorrect: 0, text: "waga 0,7"),
    WeightCase(name: "three misses", correct: 0, incorrect: 3, text: "waga 5,0"),
    WeightCase(name: "one correct answer and three misses", correct: 1, incorrect: 3, text: "waga 2,5"),
]

@Test(arguments: weightCases)
func weightText(_ c: WeightCase) {
    #expect(item("c1", correct: c.correct, incorrect: c.incorrect).weightText == c.text)
}

@Test func theTimeOfTheNextReviewStaysWithItsPreposition() throws {
    let date = try #require(Calendar.current.date(from: DateComponents(year: 2026, month: 11, day: 14, hour: 20, minute: 33)))
    #expect(nextReviewText(date) == "Następna powtórka: 14 listopada o\u{00A0}20:33")
}
