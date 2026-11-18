import Foundation

/// The strip above the card of the Nauka tab: the card on screen, then the next due cards.
struct QueueStrip: Equatable {
    enum Segment: Equatable {
        case current, good, almost, miss, heavy, waiting
    }

    let segments: [Segment]
    /// The number of due cards that have no segment.
    let more: Int

    init(card: Components.Schemas.StudyCard, grade: Components.Schemas.Grade?, queue: [Components.Schemas.StudyCard], due: Int) {
        let first: Segment =
            switch grade {
            case nil: .current
            case .correct: .good
            case .almost: .almost
            case .again: .miss
            }
        // After a failed load the queue still holds the answered card.
        let others = queue.filter { $0.card.id != card.card.id }
        let next = others.prefix(9)
        segments = [first] + next.map { $0.incorrect > $0.correct ? .heavy : .waiting }
        more = due - (queue.count - others.count) - next.count
    }
}

extension Components.Schemas.StudyCard {
    /// The weight of the card after one more miss.
    var weightText: String {
        let weight = Double(incorrect + 2) / Double(correct + 1)
        return "waga " + weight.formatted(.number.precision(.fractionLength(1)).locale(Locale(identifier: "pl_PL")))
    }
}
