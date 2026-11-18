import SwiftUI

/// Two modes, or the text for a user with no group. The mode `Słówko` has the field of a new word, its AI draft, the meanings of the draft and the similar cards of the group.
/// The mode `Gramatyka` has the field of a text, its grammar check and the row that opens the cards of the check.
struct AddView: View {
    let profile: Profile
    let cards: Cards
    let draft: CardDraft
    let check: GrammarCheck
    let offerConsent: () -> Void

    @State private var mode = Mode.word
    @State private var text = ""
    @State private var error: String?
    @State private var saved: Components.Schemas.Card?
    @State private var similar: Similar?
    @State private var grammarText = ""
    @State private var grammarSaved = false
    @State private var checking = false

    private enum Mode {
        case word, grammar
    }

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

    /// A new check cannot replace the cards of a check that are not saved or removed.
    private var canCheck: Bool { fits(grammarText, 500) && !check.waiting && check.cards.isEmpty }

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
        .navigationTitle("Dodaj")
        // The destination sits outside the Form and outside the branch for no group, so a change of the branch does not close the check screen.
        .navigationDestination(item: checked) {
            CardCheckView(fields: $0, cards: cards) { card in
                text = ""
                saved = card
                draft.reset()
            }
        }
        .navigationDestination(for: Components.Schemas.Card.self) { CardEditView(card: $0, cards: cards) }
        // The check screen shows only while the model has a card, so it closes after the save or the removal of the last card.
        .navigationDestination(isPresented: Binding(get: { checking && !check.cards.isEmpty }, set: { checking = $0 })) {
            GrammarCheckView(check: check) {
                grammarText = ""
                grammarSaved = true
            }
        }
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
        .onChange(of: grammarText) { _, new in
            check.reset()
            error = nil
            if !new.isEmpty { grammarSaved = false }
        }
        .onChange(of: mode) { error = nil }
    }

    private var form: some View {
        ThemedForm {
            Picker("Dodaj", selection: $mode) {
                Text("Słówko").tag(Mode.word)
                Text("Gramatyka").tag(Mode.grammar)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .disabled(waiting || check.waiting)
            switch mode {
            case .word:
                wordSections
            case .grammar:
                grammarSections
            }
        }
    }

    @ViewBuilder private var wordSections: some View {
        Section {
            TextField("Słowo lub wyrażenie", text: $text)
                // The keyboard applies a pending correction when a card form opens, and that changes the text and its similar cards.
                .autocorrectionDisabled()
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
                    .primaryButtonRow()
            }
        }
        // The primary button is the only row of its section, so the other rows have their own section.
        if waiting || saved != nil || error != nil {
            Section {
                if waiting {
                    ProgressView("Proszę czekać…")
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

    @ViewBuilder private var grammarSections: some View {
        Section {
            TextField("Jak tego użyłeś", text: $grammarText, axis: .vertical)
                // The mistakes of the learner are the input of the check, so the keyboard must not correct them.
                .autocorrectionDisabled()
                .disabled(check.waiting)
        } header: {
            Text("Jak tego użyłeś")
        } footer: {
            HStack(alignment: .firstTextBaseline) {
                Text("Wpisz słowo, wyrażenie albo zdanie po polsku.")
                Spacer()
                Text("\(trimmed(grammarText).unicodeScalars.count)/500").monospacedDigit()
            }
        }
        Section {
            if profile.me?.aiConsentAt == nil {
                Text("Do przygotowania fiszki potrzebna jest zgoda na AI.")
                Button("Udziel zgody", action: offerConsent)
            } else {
                Button("Sprawdź gramatykę") { requestCheck() }
                    .disabled(!canCheck)
                    .primaryButtonRow()
            }
        }
        if check.waiting || !check.cards.isEmpty || error != nil || check.clean || grammarSaved {
            Section {
                if check.waiting {
                    ProgressView("Sprawdzanie gramatyki…")
                    Text("To może potrwać kilka minut.").foregroundStyle(.secondary)
                }
                if !check.cards.isEmpty {
                    Button("Sprawdź fiszki") { checking = true }
                }
            } footer: {
                if let error {
                    Text(error).foregroundStyle(.red)
                } else if check.clean {
                    Text("Nie znaleziono błędów gramatycznych.")
                } else if grammarSaved {
                    Text("Zapisano.")
                }
            }
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

    /// The request continues when the learner changes the tab, so the check screen is open when the learner comes back.
    private func requestCheck() {
        error = nil
        Task {
            do {
                try await check.request(text: grammarText)
                checking = true
            } catch {
                self.error = errorMessage(error)
            }
        }
    }
}
