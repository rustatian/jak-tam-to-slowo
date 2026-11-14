import SwiftUI

/// The field of a new word, its AI draft, the meanings of the draft and the similar cards of the group, or the text for a user with no group.
struct AddView: View {
    let profile: Profile
    let cards: Cards
    let draft: CardDraft
    let offerConsent: () -> Void

    @State private var text = ""
    @State private var error: String?
    @State private var saved: Components.Schemas.Card?
    @State private var similar: Similar?

    /// The last answer of the search of similar cards.
    private enum Similar {
        case cards([Components.Schemas.Card])
        case failed
    }

    private struct SearchKey: Equatable {
        let groupId: String?
        let query: String
    }

    /// The text of the field as the model sends it, so that a space at an end does not start a search.
    private var query: String { text.trimmingCharacters(in: .whitespacesAndNewlines) }

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
            CardCheckView(fields: $0, cards: cards) { card in
                text = ""
                saved = card
                draft.reset()
            }
        }
        .navigationDestination(for: Components.Schemas.Card.self) { CardEditView(card: $0, cards: cards) }
        .task(id: SearchKey(groupId: profile.me?.groupId, query: query)) {
            guard !query.isEmpty else {
                similar = nil
                return
            }
            // A new text cancels this task, so a search starts 300 ms after the last key, and the answer of a cancelled search changes nothing.
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            do {
                let found = try await cards.similar(to: query)
                guard !Task.isCancelled else { return }
                similar = .cards(found)
            } catch {
                guard !Task.isCancelled else { return }
                similar = .failed
            }
        }
        // The save clears the field, so an empty text keeps the line "Fiszka zapisana." and the link to the card.
        .onChange(of: text) { _, new in
            draft.reset()
            error = nil
            if !new.isEmpty { saved = nil }
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
                if let saved {
                    NavigationLink("Otwórz fiszkę", value: saved)
                }
            } footer: {
                if let error {
                    Text(error).foregroundStyle(.red)
                } else if saved != nil {
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
            similarSection
        }
    }

    @ViewBuilder private var similarSection: some View {
        switch similar {
        case let .cards(found) where !found.isEmpty:
            Section("Już w kolekcji") {
                ForEach(found, id: \.id) { card in
                    NavigationLink(value: card) {
                        Text(card.pl + " / " + card.ru)
                    }
                    // The draft cannot end under an open card form.
                    .disabled(waiting)
                }
            }
        case .cards:
            Section {
                Text("Brak podobnych słówek.").foregroundStyle(.secondary)
            }
        case .failed:
            Section {
                Text("Nie udało się wyszukać podobnych słówek. Sprawdź kolekcję na karcie „Słówka”.").foregroundStyle(.secondary)
            }
        case nil:
            EmptyView()
        }
    }

    private func request(meaning: String? = nil) {
        error = nil
        Task {
            do {
                try await draft.request(text: text, meaning: meaning)
            } catch {
                self.error = errorMessage(error)
            }
        }
    }
}
