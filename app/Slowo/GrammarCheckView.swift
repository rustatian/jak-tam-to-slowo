import SwiftUI

/// The cards of a grammar check before the save: the fields of each card, its rule behind a fold, the removal of a card, and the save of the cards that are left.
struct GrammarCheckView: View {
    @Bindable var check: GrammarCheck
    /// Runs after the save of the last card.
    let saved: () -> Void

    @State private var working = false
    @State private var error = ""
    @State private var failed = false

    var body: some View {
        Form {
            ForEach($check.cards) { $card in
                Section {
                    LabeledContent("Twoja forma", value: card.grammar.used)
                    LabeledContent("Poprawnie", value: card.grammar.right)
                    LabeledContent("Odpowiedź") {
                        TextField("Odpowiedź", text: $card.pl)
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                            .disabled(working)
                    }
                    // A LabeledContent row keeps the height of one line, so it cuts the wrapped sentence.
                    HStack(alignment: .firstTextBaseline) {
                        Text("Zdanie z luką")
                        TextField("Zdanie z luką", text: $card.examplePl, axis: .vertical)
                            .autocorrectionDisabled()
                            .disabled(working)
                    }
                    LabeledContent("Podpowiedź", value: card.grammar.hint)
                    if !card.alternativesPl.isEmpty {
                        Text("Inne poprawne odpowiedzi: " + card.alternativesPl.joined(separator: ", ")).foregroundStyle(.secondary)
                    }
                    DisclosureGroup("Reguła i tabela") {
                        Text(card.grammar.rule)
                        // Texts can repeat, so the position identifies each item.
                        ForEach(Array(card.grammar.examples.enumerated()), id: \.offset) { _, example in
                            Text(example)
                        }
                        if let table = card.grammar.table {
                            VStack(alignment: .leading) {
                                Text(table.title).font(.caption)
                                GrammarTableView(table: table)
                            }
                        }
                    }
                    Button("Usuń fiszkę", role: .destructive) { check.remove(card.id) }
                        .disabled(working)
                } header: {
                    Text(card.grammar.topic)
                } footer: {
                    if !card.hasOneGap {
                        Text("Zdanie musi zawierać lukę ___ dokładnie raz.").foregroundStyle(.red)
                    }
                }
            }
        }
        .alert(error, isPresented: $failed) {}
        .navigationTitle("Sprawdź fiszki")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button("Zapisz") { Task { await save() } }
                    .disabled(working || !check.cards.allSatisfy(\.isValid))
            }
        }
    }

    /// A save with no error leaves no card. This is also true when the user is not a member of the group any more: `AddView` then shows the text for no group.
    private func save() async {
        working = true
        defer { working = false }
        do {
            try await check.save()
            saved()
        } catch {
            self.error = errorMessage(error)
            failed = true
        }
    }
}
