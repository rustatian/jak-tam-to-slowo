import Foundation

/// The texts of a translation card in a form. Each list of other terms is one text with one term in each line.
struct CardFields: Equatable {
    var pl: String
    var ru: String
    var meaningRu: String
    var examplePl: String
    var exampleRu: String
    var alternativesPl: String
    var alternativesRu: String

    init(_ card: Components.Schemas.Card) {
        pl = card.pl
        ru = card.ru
        meaningRu = card.meaningRu
        examplePl = card.examplePl
        exampleRu = card.exampleRu
        alternativesPl = card.alternativesPl.joined(separator: "\n")
        alternativesRu = card.alternativesRu.joined(separator: "\n")
    }

    /// The text rules of the server: `pl`, `ru` and `meaningRu` have 1 to 200 code points after the trim,
    /// the examples have 1 to 500, and each list has at most 8 terms of 1 to 200.
    var isValid: Bool {
        fits(pl, 200) && fits(ru, 200) && fits(meaningRu, 200) && fits(examplePl, 500) && fits(exampleRu, 500)
            && fitsTerms(alternativesPl) && fitsTerms(alternativesRu)
    }

    /// The trimmed texts and the term lists that differ from `original`, or nil when nothing differs.
    func changes(from original: CardFields) -> Components.Schemas.CardUpdate? {
        func text(_ field: KeyPath<CardFields, String>) -> String? {
            let new = trimmed(self[keyPath: field])
            return new == trimmed(original[keyPath: field]) ? nil : new
        }
        // A list with no terms goes as [], because an absent list keeps the old terms on the server.
        func list(_ field: KeyPath<CardFields, String>) -> [String]? {
            let new = terms(self[keyPath: field])
            return new == terms(original[keyPath: field]) ? nil : new
        }
        let update = Components.Schemas.CardUpdate(
            pl: text(\.pl), ru: text(\.ru), examplePl: text(\.examplePl), exampleRu: text(\.exampleRu), meaningRu: text(\.meaningRu),
            alternativesPl: list(\.alternativesPl), alternativesRu: list(\.alternativesRu)
        )
        return update == .init() ? nil : update
    }
}

private func trimmed(_ text: String) -> String {
    text.trimmingCharacters(in: .whitespacesAndNewlines)
}

/// The trimmed lines that are not empty. `Character.isNewline` also matches a pasted CR LF.
private func terms(_ text: String) -> [String] {
    text.split(whereSeparator: \.isNewline).map { trimmed(String($0)) }.filter { !$0.isEmpty }
}

/// The server counts code points, so `String.count` (grapheme clusters) would pass texts that it refuses.
private func fits(_ text: String, _ max: Int) -> Bool {
    (1...max).contains(trimmed(text).unicodeScalars.count)
}

private func fitsTerms(_ text: String) -> Bool {
    let list = terms(text)
    return list.count <= 8 && list.allSatisfy { fits($0, 200) }
}
