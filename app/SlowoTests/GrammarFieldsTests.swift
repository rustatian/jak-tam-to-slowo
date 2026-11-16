import Testing

@testable import Slowo

/// A card of a grammar check, with one other accepted answer.
let psyDraft = Components.Schemas.CardCreate(
    pl: "psy", ru: "", examplePl: "Na podwórku biegają dwa ___.", exampleRu: "", meaningRu: "", alternativesPl: ["pieski"], alternativesRu: [],
    kind: .grammar, grammar: psy.grammar
)
let psySentence = "Na podwórku biegają dwa ___."

struct GrammarFieldsCase: Sendable, CustomTestStringConvertible {
    let name: String
    let pl: String
    let examplePl: String
    let hasOneGap: Bool
    let isValid: Bool
    let alternativesPl: [String]
    let createPl: String
    let createExamplePl: String

    var testDescription: String { name }
}

let grammarFieldsCases: [GrammarFieldsCase] = [
    GrammarFieldsCase(
        name: "the card of the check", pl: "psy", examplePl: psySentence, hasOneGap: true, isValid: true, alternativesPl: ["pieski"],
        createPl: "psy", createExamplePl: psySentence),
    GrammarFieldsCase(
        name: "a changed answer has no other answers", pl: "psiaki", examplePl: psySentence, hasOneGap: true, isValid: true, alternativesPl: [],
        createPl: "psiaki", createExamplePl: psySentence),
    GrammarFieldsCase(
        name: "outer spaces are no change of the answer", pl: " psy \n", examplePl: psySentence, hasOneGap: true, isValid: true,
        alternativesPl: ["pieski"], createPl: "psy", createExamplePl: psySentence),
    GrammarFieldsCase(
        name: "an empty answer", pl: "  ", examplePl: psySentence, hasOneGap: true, isValid: false, alternativesPl: [],
        createPl: "", createExamplePl: psySentence),
    GrammarFieldsCase(
        name: "an answer of 200 code points", pl: String(repeating: "ż", count: 200), examplePl: psySentence, hasOneGap: true, isValid: true,
        alternativesPl: [], createPl: String(repeating: "ż", count: 200), createExamplePl: psySentence),
    GrammarFieldsCase(
        name: "an answer of 201 code points", pl: String(repeating: "ż", count: 201), examplePl: psySentence, hasOneGap: true, isValid: false,
        alternativesPl: [], createPl: String(repeating: "ż", count: 201), createExamplePl: psySentence),
    GrammarFieldsCase(
        name: "a sentence with no gap", pl: "psy", examplePl: "Na podwórku biegają dwa psy.", hasOneGap: false, isValid: false,
        alternativesPl: ["pieski"], createPl: "psy", createExamplePl: "Na podwórku biegają dwa psy."),
    GrammarFieldsCase(
        name: "a sentence with two gaps", pl: "psy", examplePl: "___ biegają dwa ___.", hasOneGap: false, isValid: false,
        alternativesPl: ["pieski"], createPl: "psy", createExamplePl: "___ biegają dwa ___."),
    GrammarFieldsCase(
        name: "four underscores are one gap", pl: "psy", examplePl: "Na podwórku biegają dwa ____.", hasOneGap: true, isValid: true,
        alternativesPl: ["pieski"], createPl: "psy", createExamplePl: "Na podwórku biegają dwa ____."),
    GrammarFieldsCase(
        name: "six underscores are two gaps", pl: "psy", examplePl: "Na podwórku biegają dwa ______.", hasOneGap: false, isValid: false,
        alternativesPl: ["pieski"], createPl: "psy", createExamplePl: "Na podwórku biegają dwa ______."),
    GrammarFieldsCase(
        name: "a sentence of 500 code points", pl: "psy", examplePl: String(repeating: "a", count: 497) + "___", hasOneGap: true,
        isValid: true, alternativesPl: ["pieski"], createPl: "psy", createExamplePl: String(repeating: "a", count: 497) + "___"),
    GrammarFieldsCase(
        name: "a sentence of 501 code points", pl: "psy", examplePl: String(repeating: "a", count: 498) + "___", hasOneGap: true,
        isValid: false, alternativesPl: ["pieski"], createPl: "psy", createExamplePl: String(repeating: "a", count: 498) + "___"),
    GrammarFieldsCase(
        name: "the sentence goes out trimmed", pl: "psy", examplePl: " " + psySentence + "\n", hasOneGap: true, isValid: true,
        alternativesPl: ["pieski"], createPl: "psy", createExamplePl: psySentence),
]

@Test(arguments: grammarFieldsCases)
func grammarFields(_ c: GrammarFieldsCase) throws {
    var fields = try #require(GrammarFields(id: 0, psyDraft))
    fields.pl = c.pl
    fields.examplePl = c.examplePl
    #expect(fields.hasOneGap == c.hasOneGap)
    #expect(fields.isValid == c.isValid)
    #expect(fields.alternativesPl == c.alternativesPl)
    var body = psyDraft
    body.pl = c.createPl
    body.examplePl = c.createExamplePl
    body.alternativesPl = c.alternativesPl
    #expect(fields.create == body)
}

@Test func aCardWithNoGrammarMemberGivesNoFields() {
    #expect(GrammarFields(id: 0, domDraft) == nil)
}
