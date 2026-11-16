import Testing

@testable import Slowo

struct GapCase: Sendable, CustomTestStringConvertible {
    let name: String
    let examplePl: String
    let before: String
    let after: String

    var testDescription: String { name }
}

let gapCases: [GapCase] = [
    GapCase(name: "a gap in the middle", examplePl: "Idę na spacer z ___ do parku.", before: "Idę na spacer z ", after: " do parku."),
    GapCase(name: "a gap at the start", examplePl: "___ się Ola Nowak.", before: "", after: " się Ola Nowak."),
    GapCase(name: "a gap at the end", examplePl: "Na podwórku biegają dwa ___", before: "Na podwórku biegają dwa ", after: ""),
    GapCase(name: "a run of five underscores is one gap", examplePl: "Wczoraj _____ list.", before: "Wczoraj ", after: " list."),
]

@Test(arguments: gapCases)
func gapSentence(_ c: GapCase) {
    let gap = GapSentence(c.examplePl)
    #expect(gap.before == c.before)
    #expect(gap.after == c.after)
}

struct GapFeedbackCase: Sendable, CustomTestStringConvertible {
    let name: String
    let answer: String
    let grade: Components.Schemas.Grade
    let correction: String
    let text: String
    let cardForm: String?

    var testDescription: String { name }
}

let gapFeedbackCases: [GapFeedbackCase] = [
    GapFeedbackCase(name: "again shows the correction", answer: "piesy", grade: .again, correction: "psy", text: "psy", cardForm: nil),
    GapFeedbackCase(name: "an empty answer shows the correction", answer: "", grade: .again, correction: "psy", text: "psy", cardForm: nil),
    GapFeedbackCase(name: "almost shows the correction", answer: "kolezanka", grade: .almost, correction: "koleżanką", text: "koleżanką", cardForm: nil),
    GapFeedbackCase(name: "correct with the text of the card shows the answer", answer: "psy", grade: .correct, correction: "psy", text: "psy", cardForm: nil),
    GapFeedbackCase(
        name: "correct with another letter case and outer spaces names no form of the card", answer: " Psy ", grade: .correct, correction: "psy", text: "Psy",
        cardForm: nil
    ),
    GapFeedbackCase(name: "correct with another form names the form of the card", answer: "psami", grade: .correct, correction: "psem", text: "psami", cardForm: "psem"),
]

@Test(arguments: gapFeedbackCases)
func gapFeedback(_ c: GapFeedbackCase) {
    let feedback = GapFeedback(answer: c.answer, review: graded(c.grade, c.correction))
    #expect(feedback.text == c.text)
    #expect(feedback.cardForm == c.cardForm)
}
