import Foundation

/// A grammar card of a check on the check screen: the card as the check gave it, and the two texts that the learner can change.
struct GrammarFields: Identifiable, Equatable {
    /// The position of the card in the result of the check.
    let id: Int
    let grammar: Components.Schemas.Grammar
    var pl: String
    var examplePl: String
    private let card: Components.Schemas.CardCreate

    /// Nil for a card with no `grammar` member.
    init?(id: Int, _ card: Components.Schemas.CardCreate) {
        guard let grammar = card.grammar else { return nil }
        self.id = id
        self.grammar = grammar
        self.card = card
        pl = card.pl
        examplePl = card.examplePl
    }

    /// The other accepted answers of the check. The list is empty when the trimmed `pl` is not the `pl` of the check.
    var alternativesPl: [String] {
        trimmed(pl) == card.pl ? card.alternativesPl ?? [] : []
    }

    /// The trimmed `examplePl` has `___` exactly once, as non-overlapping matches.
    var hasOneGap: Bool {
        trimmed(examplePl).components(separatedBy: "___").count == 2
    }

    /// `pl` has 1 to 200 code points, `examplePl` has 1 to 500, and `hasOneGap`.
    var isValid: Bool {
        fits(pl, 200) && fits(examplePl, 500) && hasOneGap
    }

    /// The card of the check with the trimmed `pl` and `examplePl`, and with `alternativesPl`.
    var create: Components.Schemas.CardCreate {
        var body = card
        body.pl = trimmed(pl)
        body.examplePl = trimmed(examplePl)
        body.alternativesPl = alternativesPl
        return body
    }
}
