import Testing

@testable import Slowo

@Test func fieldsOfACard() {
    var card = dom
    card.alternativesPl = ["Domek", "Chata"]
    let fields = CardFields(card)

    #expect(fields.pl == "Dom")
    #expect(fields.ru == "Дом")
    #expect(fields.meaningRu == "здание")
    #expect(fields.examplePl == "To jest dom.")
    #expect(fields.exampleRu == "Это дом.")
    #expect(fields.alternativesPl == "Domek\nChata")
    #expect(fields.alternativesRu == "")
}

struct CardValidCase: Sendable, CustomTestStringConvertible {
    let name: String
    let change: @Sendable (inout CardFields) -> Void
    let valid: Bool

    var testDescription: String { name }
}

let validCases: [CardValidCase] = [
    CardValidCase(name: "the base card is valid", change: { _ in }, valid: true),
    CardValidCase(name: "a blank Polish term", change: { $0.pl = "  " }, valid: false),
    CardValidCase(name: "an empty Russian term", change: { $0.ru = "" }, valid: false),
    CardValidCase(name: "a meaning of a line break", change: { $0.meaningRu = " \n" }, valid: false),
    CardValidCase(name: "an empty Polish example", change: { $0.examplePl = "" }, valid: false),
    CardValidCase(name: "an empty Russian example", change: { $0.exampleRu = "" }, valid: false),
    CardValidCase(name: "200 letters of a term pass", change: { $0.pl = String(repeating: "ż", count: 200) }, valid: true),
    CardValidCase(name: "201 letters of a term are refused", change: { $0.pl = String(repeating: "ż", count: 201) }, valid: false),
    CardValidCase(name: "201 letters of the Russian term are refused", change: { $0.ru = String(repeating: "я", count: 201) }, valid: false),
    CardValidCase(name: "200 letters of a meaning pass", change: { $0.meaningRu = String(repeating: "я", count: 200) }, valid: true),
    CardValidCase(name: "spaces at the ends do not count", change: { $0.ru = " " + String(repeating: "я", count: 200) + " " }, valid: true),
    CardValidCase(name: "a letter of two UTF-16 units counts one time", change: { $0.ru = String(repeating: "\u{1D538}", count: 200) }, valid: true),
    CardValidCase(name: "a combining mark counts as a code point", change: { $0.meaningRu = String(repeating: "e\u{0301}", count: 101) }, valid: false),
    CardValidCase(name: "500 letters of the Polish example pass", change: { $0.examplePl = String(repeating: "a", count: 500) }, valid: true),
    CardValidCase(name: "501 letters of the Polish example are refused", change: { $0.examplePl = String(repeating: "a", count: 501) }, valid: false),
    CardValidCase(name: "500 letters of the Russian example pass", change: { $0.exampleRu = String(repeating: "я", count: 500) }, valid: true),
    CardValidCase(name: "501 letters of the Russian example are refused", change: { $0.exampleRu = String(repeating: "я", count: 501) }, valid: false),
    CardValidCase(name: "8 terms pass", change: { $0.alternativesPl = "a1\na2\na3\na4\na5\na6\na7\na8" }, valid: true),
    CardValidCase(name: "9 terms are refused", change: { $0.alternativesRu = "a1\na2\na3\na4\na5\na6\na7\na8\na9" }, valid: false),
    CardValidCase(name: "blank lines are no terms", change: { $0.alternativesPl = "\na1\n\na2\n\na3\n\na4\n\na5\n\na6\n\na7\n\na8\n   " }, valid: true),
    CardValidCase(name: "a term of 201 letters is refused", change: { $0.alternativesRu = String(repeating: "a", count: 201) }, valid: false),
]

@Test(arguments: validCases)
func cardFieldsRule(_ c: CardValidCase) {
    var fields = CardFields(dom)
    c.change(&fields)

    #expect(fields.isValid == c.valid)
}

struct CardChangeCase: Sendable, CustomTestStringConvertible {
    let name: String
    let change: @Sendable (inout CardFields) -> Void
    let update: Components.Schemas.CardUpdate?

    var testDescription: String { name }
}

let changeCases: [CardChangeCase] = [
    CardChangeCase(name: "no change gives no update", change: { _ in }, update: nil),
    CardChangeCase(name: "spaces at the ends give no update", change: { $0.pl = " Dom " }, update: nil),
    CardChangeCase(name: "blank lines in a list give no update", change: { $0.alternativesPl = "\nDomek\n\n" }, update: nil),
    CardChangeCase(name: "one changed text", change: { $0.pl = "Domek" }, update: .init(pl: "Domek")),
    CardChangeCase(name: "a changed text is trimmed", change: { $0.examplePl = " To mój dom. " }, update: .init(examplePl: "To mój dom.")),
    CardChangeCase(
        name: "two changed texts",
        change: {
            $0.ru = "Домик"
            $0.meaningRu = "маленький дом"
        },
        update: .init(ru: "Домик", meaningRu: "маленький дом")
    ),
    CardChangeCase(name: "a new term in a list", change: { $0.alternativesPl = "Domek\n Chata " }, update: .init(alternativesPl: ["Domek", "Chata"])),
    CardChangeCase(name: "a line end of two characters ends a term", change: { $0.alternativesPl = "Domek\r\nChata" }, update: .init(alternativesPl: ["Domek", "Chata"])),
    CardChangeCase(name: "a first term in an empty list", change: { $0.alternativesRu = "Домик" }, update: .init(alternativesRu: ["Домик"])),
    CardChangeCase(name: "a list that is now empty", change: { $0.alternativesPl = "" }, update: .init(alternativesPl: [])),
]

@Test(arguments: changeCases)
func cardFieldsChange(_ c: CardChangeCase) {
    let base = CardFields(dom)
    var fields = base
    c.change(&fields)

    #expect(fields.changes(from: base) == c.update)
}
