import SwiftUI

/// The prompt of a grammar card: the gap sentence with the hint after the gap.
struct GrammarPrompt: View {
    let examplePl: String
    let hint: String

    var body: some View {
        Text(sentence).font(.title3)
    }

    private var sentence: AttributedString {
        let gap = GapSentence(examplePl)
        var hintText = AttributedString(hint)
        hintText.foregroundColor = .secondary
        return AttributedString(gap.before + "___ ") + hintText + AttributedString(gap.after)
    }
}

/// The answer part of the feedback of a grammar card: the sentence with the gap filled, the form of the card, and the explanation of the model.
struct GrammarAnswer: View {
    let examplePl: String
    let answer: String
    let review: Components.Schemas.Review

    var body: some View {
        let feedback = GapFeedback(answer: answer, review: review)
        Text(sentence(feedback.text)).font(.title3)
        if let cardForm = feedback.cardForm {
            Text("Na fiszce: " + cardForm).foregroundStyle(.secondary)
        }
        if let explanation = review.explanation {
            Text(explanation).foregroundStyle(.secondary)
        }
    }

    private func sentence(_ text: String) -> AttributedString {
        let gap = GapSentence(examplePl)
        var filled = AttributedString(text)
        filled.inlinePresentationIntent = .stronglyEmphasized
        return AttributedString(gap.before) + filled + AttributedString(gap.after)
    }
}

/// The rule of a grammar card under its topic with the examples, and the table. The body is one or two sections of a form.
struct GrammarRule: View {
    let grammar: Components.Schemas.Grammar

    var body: some View {
        Section {
            Text(grammar.rule)
            // Texts can repeat, so the position identifies each item.
            ForEach(Array(grammar.examples.enumerated()), id: \.offset) { _, example in
                Text(example)
            }
        } header: {
            Text(grammar.topic)
        }
        if let table = grammar.table {
            Section {
                GrammarTableView(table: table)
            } header: {
                Text(table.title)
            }
        }
    }
}

/// The table of a grammar card. The row of the form of the card has a heavier font weight and the tint color.
struct GrammarTableView: View {
    let table: Components.Schemas.GrammarTable

    var body: some View {
        Grid(alignment: .leadingFirstTextBaseline) {
            GridRow {
                ForEach(Array(table.columns.enumerated()), id: \.offset) { _, column in
                    Text(column)
                }
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            ForEach(Array(table.groups.enumerated()), id: \.offset) { _, group in
                if let title = group.title {
                    // A view outside a grid row takes all columns.
                    Text(title).font(.caption).foregroundStyle(.secondary)
                }
                ForEach(Array(group.rows.enumerated()), id: \.offset) { _, row in
                    GridRow {
                        Text(row.names.joined(separator: "\n"))
                        Text(row.endings)
                        Text(row.examples.joined(separator: "\n"))
                    }
                    .fontWeight(row.hit ? .semibold : nil)
                    .foregroundStyle(row.hit ? AnyShapeStyle(.tint) : AnyShapeStyle(.primary))
                }
            }
        }
        .font(.footnote)
        // With a fixed height proposal a grid cuts the texts of its cells to one line.
        .fixedSize(horizontal: false, vertical: true)
    }
}
