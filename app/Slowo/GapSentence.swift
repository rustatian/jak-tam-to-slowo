import Foundation

/// The gap sentence of a grammar card, cut at its gap. The gap is the first run of three or more underscores.
struct GapSentence {
    let before: String
    let after: String

    init(_ examplePl: String) {
        // The server refuses a grammar card with no gap, so a sentence with no match is all `before`.
        let gap = examplePl.firstRange(of: /_{3,}/) ?? examplePl.endIndex..<examplePl.endIndex
        before = String(examplePl[..<gap.lowerBound])
        after = String(examplePl[gap.upperBound...])
    }
}

/// The text for the gap in the feedback of a grammar card.
struct GapFeedback {
    /// The answer of the learner after a grade of correct, the correction after each other grade.
    let text: String
    /// The correction, when the grade is correct and the answer is another text. The comparison ignores the letter case.
    let cardForm: String?

    init(answer: String, review: Components.Schemas.Review) {
        guard review.grade == .correct else {
            text = review.correction
            cardForm = nil
            return
        }
        text = answer.trimmingCharacters(in: .whitespacesAndNewlines)
        cardForm = text.compare(review.correction, options: .caseInsensitive) == .orderedSame ? nil : review.correction
    }
}
