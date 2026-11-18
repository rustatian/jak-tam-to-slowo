import SwiftUI

/// The screen of a saved grammar card: the gap sentence with the hint, the answer, the wrong and the right form, the rule with the table, and the archive.
struct GrammarCardView: View {
    let card: Components.Schemas.Card
    let grammar: Components.Schemas.Grammar
    let cards: Cards

    @State private var working = false
    @State private var error = ""
    @State private var failed = false
    @State private var confirmArchive = false
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ThemedForm {
            Section("Zdanie") {
                GrammarPrompt(examplePl: card.examplePl, hint: grammar.hint)
            }
            Section {
                LabeledContent("Odpowiedź", value: card.pl)
                if !card.alternativesPl.isEmpty {
                    LabeledContent("Inne odpowiedzi", value: card.alternativesPl.joined(separator: ", "))
                }
            }
            Section {
                LabeledContent("Błędna forma", value: grammar.used)
                LabeledContent("Poprawnie", value: grammar.right)
            }
            GrammarRule(grammar: grammar)
            Section {
                Button("Do archiwum", role: .destructive) { confirmArchive = true }
                    .disabled(working)
                    .confirmationDialog("Przenieść fiszkę do archiwum?", isPresented: $confirmArchive, titleVisibility: .visible) {
                        Button("Do archiwum", role: .destructive) {
                            Task { await archive() }
                        }
                    } message: {
                        Text("Fiszka zniknie u wszystkich osób w grupie. Tej czynności nie można cofnąć.")
                    }
            }
        }
        .alert(error, isPresented: $failed) {}
        .navigationTitle("Fiszka")
        .navigationBarTitleDisplayMode(.inline)
    }

    /// Closes the screen after a call with no error. A call also ends with no error when the user is not a member of the group any more.
    private func archive() async {
        working = true
        defer { working = false }
        do {
            try await cards.archive(card.id)
            dismiss()
        } catch {
            self.error = errorMessage(error)
            failed = true
        }
    }
}
