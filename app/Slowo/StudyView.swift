import SwiftUI

/// The card of the study queue (a translation card or a grammar card) with the answer field and the feedback, the texts for an empty queue, or the text for a user with no group.
struct StudyView: View {
    let profile: Profile
    let study: Study

    @State private var text = ""
    @State private var working = false
    @State private var failed = false
    @State private var error: String?
    @Environment(\.scenePhase) private var scenePhase

    /// The condition of "Sprawdź" and of the Return key. An empty answer counts as a miss, so only "Pokaż odpowiedź" sends it.
    private var canCheck: Bool { fits(text, 500) && !working }

    /// The text of the field. An edit of the learner removes the error.
    /// The field also calls the setter when it gets the focus, with the same text.
    private var fieldText: Binding<String> {
        Binding {
            text
        } set: {
            if $0 != text {
                error = nil
            }
            text = $0
        }
    }

    var body: some View {
        // The ZStack keeps one identity for all states, so a change of the state does not start the task again,
        // and the modifiers on it also see the changes that occur while a branch does not show.
        ZStack {
            if profile.me?.groupId == nil {
                NoGroupView()
            } else {
                content
            }
        }
        .groundBackground()
        .navigationTitle("Nauka")
        .task(id: profile.me?.groupId) { await load() }
        .onChange(of: scenePhase) { _, phase in
            // Cards become due while the app is in the background.
            if phase == .active { Task { await load() } }
        }
        // This write does not use the binding of the field, so the error of a 404 or a 409 stays with the next card.
        .onChange(of: study.card) { text = "" }
        .sensoryFeedback(.success, trigger: study.review) { _, new in new?.grade == .correct }
    }

    @ViewBuilder private var content: some View {
        if let card = study.card, let direction = profile.me?.direction {
            form(card, direction)
        } else if study.queue != nil {
            ThemedList {}
                .overlay {
                    if let nextDueAt = study.nextDueAt {
                        ContentUnavailableView {
                            Label("Na dziś wszystko gotowe.", systemImage: "checkmark.circle")
                        } description: {
                            VStack {
                                Text(nextReviewText(nextDueAt))
                                errorText
                            }
                        }
                    } else {
                        ContentUnavailableView {
                            Label("Dodaj słówko, aby zacząć.", systemImage: "books.vertical")
                        } description: {
                            errorText
                        }
                    }
                }
                .refreshable { await load() }
        } else if failed {
            ContentUnavailableView {
                Label("Nie udało się połączyć z serwerem.", systemImage: "wifi.exclamationmark")
            } actions: {
                Button("Spróbuj ponownie") { Task { await load() } }
            }
        } else {
            ProgressView()
        }
    }

    private func form(_ card: Components.Schemas.StudyCard, _ direction: Components.Schemas.Direction) -> some View {
        let grammar = card.card.grammar
        // The answer of a grammar card is a Polish form in both directions.
        let header: LocalizedStringKey = grammar != nil || direction == .ruPl ? "Odpowiedź po polsku" : "Odpowiedź po rosyjsku"
        return ThemedForm {
            Section {
                if let grammar {
                    GrammarPrompt(examplePl: card.card.examplePl, hint: grammar.hint)
                } else {
                    VStack(alignment: .leading) {
                        switch direction {
                        case .ruPl:
                            Text(card.card.ru).font(.title)
                            Text(card.card.meaningRu).foregroundStyle(.secondary)
                            Text(card.card.exampleRu)
                        case .plRu:
                            Text(card.card.pl).font(.title)
                            Text(card.card.examplePl)
                        }
                    }
                }
            }
            Section {
                TextField(header, text: fieldText)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .disabled(working || study.review != nil)
                    .onSubmit { if canCheck { send(text) } }
            } header: {
                Text(header)
            }
            if let review = study.review {
                Section {
                    VStack(alignment: .leading) {
                        switch review.grade {
                        case .correct: Label("Dobrze", systemImage: "checkmark.circle.fill").foregroundStyle(.green)
                        case .almost: Label("Prawie dobrze", systemImage: "exclamationmark.circle.fill").foregroundStyle(.orange)
                        case .again: Label("Powtórz", systemImage: "xmark.circle.fill").foregroundStyle(.red)
                        }
                        if grammar != nil {
                            GrammarAnswer(examplePl: card.card.examplePl, answer: text, review: review)
                        } else {
                            Text(review.correction).font(.title2)
                            Text(direction == .ruPl ? card.card.examplePl : card.card.exampleRu)
                            if let explanation = review.explanation {
                                Text(explanation).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
                if let grammar {
                    GrammarRule(grammar: grammar)
                }
            }
            Section {
                if study.review == nil {
                    Button("Sprawdź") { send(text) }
                        .disabled(!canCheck)
                    Button("Pokaż odpowiedź") {
                        text = ""
                        send("")
                    }
                    .disabled(working)
                    if working {
                        ProgressView("Sprawdzanie odpowiedzi…")
                    }
                } else {
                    // Always active: a tap during the reload inside `answer` shows the next card with the progress view until the call ends.
                    Button("Dalej") { study.next() }
                }
            } footer: {
                errorText
            }
        }
        .refreshable { await load() }
    }

    @ViewBuilder private var errorText: some View {
        if let error {
            Text(error).foregroundStyle(.red)
        }
    }

    private func load() async {
        failed = false
        do {
            try await study.load()
            error = nil
        } catch {
            // The view went away or the group ID changed: the load did not fail.
            guard !Task.isCancelled else { return }
            if study.queue == nil {
                failed = true
            } else {
                self.error = errorMessage(error)
            }
        }
    }

    /// Sends the answer to the card on screen. The call includes the reload of the queue, so `working` stays on until the reload ends.
    private func send(_ answer: String) {
        error = nil
        working = true
        Task {
            defer { working = false }
            do {
                try await study.answer(answer)
            } catch {
                self.error = errorMessage(error)
            }
        }
    }
}

/// The text of the next review on the "all done" state. A no-break space keeps the time on the line of its preposition.
func nextReviewText(_ nextDueAt: Date) -> String {
    var text = "Następna powtórka: " + nextDueAt.formatted(.dateTime.day().month(.wide).hour().minute().locale(Locale(identifier: "pl_PL")))
    if let space = text.lastIndex(of: " ") {
        text.replaceSubrange(space...space, with: "\u{00A0}")
    }
    return text
}
