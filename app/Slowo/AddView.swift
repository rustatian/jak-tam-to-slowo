import SwiftUI

/// The field of a new word, its AI draft and the meanings of the draft, or the text for a user with no group.
struct AddView: View {
    let profile: Profile
    let cards: Cards
    let draft: CardDraft
    let offerConsent: () -> Void

    @State private var text = ""
    @State private var error: String?
    @State private var saved = false

    private var waiting: Bool { draft.step == .waiting }

    /// The condition of the button and of the Return key. The field shows also without the AI consent.
    private var canRequest: Bool { profile.me?.aiConsentAt != nil && fits(text, 200) && !waiting }

    /// The fields of a draft card. Back from the check screen sets nil, which drops the draft.
    private var checked: Binding<CardFields?> {
        Binding {
            if case let .card(fields) = draft.step { fields } else { nil }
        } set: {
            if $0 == nil { draft.reset() }
        }
    }

    var body: some View {
        ZStack {
            if profile.me?.groupId == nil {
                NoGroupView()
            } else {
                form
            }
        }
        .navigationTitle("Dodaj słówko")
        // The destination sits outside the Form and outside the branch for no group, so a change of the branch does not close the check screen.
        .navigationDestination(item: checked) {
            CardCheckView(fields: $0, cards: cards) {
                text = ""
                saved = true
                draft.reset()
            }
        }
        // The save clears the field, so an empty text keeps the line "Fiszka zapisana.".
        .onChange(of: text) { _, new in
            draft.reset()
            error = nil
            if !new.isEmpty { saved = false }
        }
    }

    private var form: some View {
        Form {
            Section {
                TextField("Słowo lub wyrażenie", text: $text)
                    .disabled(waiting)
                    .onSubmit { if canRequest { request() } }
            } header: {
                Text("Słowo lub wyrażenie")
            } footer: {
                Text("Wpisz słowo po polsku albo po rosyjsku.")
            }
            Section {
                if profile.me?.aiConsentAt == nil {
                    Text("Do przygotowania fiszki potrzebna jest zgoda na AI.")
                    Button("Udziel zgody", action: offerConsent)
                } else {
                    Button("Przygotuj fiszkę") { request() }
                        .disabled(!canRequest)
                    if waiting {
                        ProgressView("Proszę czekać…")
                    }
                }
            } footer: {
                if let error {
                    Text(error).foregroundStyle(.red)
                } else if saved {
                    Text("Fiszka zapisana.")
                }
            }
            if case let .choices(meanings) = draft.step {
                Section("Wybierz znaczenie") {
                    ForEach(meanings, id: \.self) { meaning in
                        Button(meaning) { request(meaning: meaning) }
                    }
                }
            }
        }
    }

    private func request(meaning: String? = nil) {
        error = nil
        saved = false
        Task {
            do {
                try await draft.request(text: text, meaning: meaning)
            } catch {
                self.error = errorMessage(error)
            }
        }
    }
}
