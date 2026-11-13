import SwiftUI

/// The sections of the seven fields of a translation card and the hint of the lists, for use inside a `Form`.
struct CardFieldsForm: View {
    @Binding var fields: CardFields

    var body: some View {
        Section("Po polsku") { TextField("Po polsku", text: $fields.pl) }
        Section("Po rosyjsku") { TextField("Po rosyjsku", text: $fields.ru) }
        Section("Znaczenie (po rosyjsku)") { TextField("Znaczenie (po rosyjsku)", text: $fields.meaningRu) }
        Section("Przykład po polsku") { TextField("Przykład po polsku", text: $fields.examplePl, axis: .vertical) }
        Section("Przykład po rosyjsku") { TextField("Przykład po rosyjsku", text: $fields.exampleRu, axis: .vertical) }
        Section("Inne odpowiedzi po polsku") { TextField("Inne odpowiedzi po polsku", text: $fields.alternativesPl, axis: .vertical) }
        Section {
            TextField("Inne odpowiedzi po rosyjsku", text: $fields.alternativesRu, axis: .vertical)
        } header: {
            Text("Inne odpowiedzi po rosyjsku")
        } footer: {
            Text("Inne poprawne odpowiedzi: do ośmiu dla każdego języka, jedna w wierszu. Do 200 znaków na odpowiedź.")
        }
    }
}

/// The form of a saved translation card: a save of the changed fields, and the archive.
struct CardEditView: View {
    let card: Components.Schemas.Card
    let cards: Cards

    @State private var fields: CardFields
    @State private var working = false
    @State private var error = ""
    @State private var failed = false
    @State private var confirmArchive = false
    @Environment(\.dismiss) private var dismiss

    init(card: Components.Schemas.Card, cards: Cards) {
        self.card = card
        self.cards = cards
        _fields = State(initialValue: CardFields(card))
    }

    private var change: Components.Schemas.CardUpdate? { fields.changes(from: CardFields(card)) }

    var body: some View {
        Form {
            CardFieldsForm(fields: $fields)
            Section {
                Button("Do archiwum", role: .destructive) { confirmArchive = true }
                    .disabled(working)
                    .confirmationDialog("Przenieść fiszkę do archiwum?", isPresented: $confirmArchive, titleVisibility: .visible) {
                        Button("Do archiwum", role: .destructive) {
                            Task { await run { try await cards.archive(card.id) } }
                        }
                    } message: {
                        Text("Fiszka zniknie u wszystkich osób w grupie. Tej czynności nie można cofnąć.")
                    }
            }
        }
        .alert(error, isPresented: $failed) {}
        .navigationTitle("Fiszka")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button("Zapisz") { Task { await save() } }
                    .disabled(working || !fields.isValid || change == nil)
            }
        }
    }

    private func save() async {
        guard let change else { return }
        await run { try await cards.update(card.id, change) }
    }

    /// Closes the form after a call with no error. A call also ends with no error when the user is not a member of the group any more.
    private func run(_ call: () async throws -> Void) async {
        working = true
        defer { working = false }
        do {
            try await call()
            dismiss()
        } catch {
            self.error = errorMessage(error)
            failed = true
        }
    }
}

/// The form of an AI draft: the save makes a new card.
struct CardCheckView: View {
    let cards: Cards
    let saved: () -> Void

    @State private var fields: CardFields
    @State private var working = false
    @State private var error = ""
    @State private var failed = false

    init(fields: CardFields, cards: Cards, saved: @escaping () -> Void) {
        self.cards = cards
        self.saved = saved
        _fields = State(initialValue: fields)
    }

    var body: some View {
        Form {
            CardFieldsForm(fields: $fields)
        }
        .alert(error, isPresented: $failed) {}
        .navigationTitle("Sprawdź fiszkę")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button("Zapisz") { Task { await save() } }
                    .disabled(working || !fields.isValid)
            }
        }
    }

    /// `saved` closes the form. A save also ends with no error when the user is not a member of the group any more.
    private func save() async {
        working = true
        defer { working = false }
        do {
            try await cards.create(fields.create)
            saved()
        } catch {
            self.error = errorMessage(error)
            failed = true
        }
    }
}
