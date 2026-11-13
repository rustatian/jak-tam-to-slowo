import SwiftUI

/// The translation cards of the group with a search field, or the text for a user with no group.
struct WordsView: View {
    let profile: Profile
    let cards: Cards

    @State private var search = ""
    @State private var failed = false
    @State private var error: String?

    private struct LoadKey: Equatable {
        let groupId: String?
        let query: String
    }

    /// The search text as the model sends it, so that a space at an end does not start a load.
    private var query: String { search.trimmingCharacters(in: .whitespacesAndNewlines) }

    var body: some View {
        // The ZStack keeps one identity for all states, so a change of the state does not start the task again.
        ZStack {
            if profile.me?.groupId == nil {
                NoGroupView()
            } else {
                // The search field sits on one container of the list states, so a change of the state does not end the search.
                ZStack { content }
                    .searchable(text: $search, prompt: "Szukaj słówek")
            }
        }
        .navigationTitle("Słówka")
        // The destination sits outside the List, so the form stays open when a load removes the row of its card.
        .navigationDestination(for: Components.Schemas.Card.self) { CardEditView(card: $0, cards: cards) }
        .task(id: LoadKey(groupId: profile.me?.groupId, query: query)) {
            // A new text cancels this task, so a load starts 300 ms after the last change of the text.
            if !query.isEmpty {
                try? await Task.sleep(for: .milliseconds(300))
                guard !Task.isCancelled else { return }
            }
            await load()
        }
    }

    @ViewBuilder private var content: some View {
        if let list = cards.cards {
            List {
                Section {
                    ForEach(list, id: \.id) { card in
                        NavigationLink(value: card) {
                            VStack(alignment: .leading) {
                                Text(card.pl)
                                Text(card.ru + " · " + card.meaningRu)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                } footer: {
                    // With no rows the error shows on the empty screen.
                    if !list.isEmpty { errorText }
                }
            }
            .overlay {
                if list.isEmpty {
                    if cards.query.isEmpty {
                        ContentUnavailableView {
                            Label("Nie ma jeszcze słówek.", systemImage: "books.vertical")
                        } description: {
                            VStack {
                                Text("Dodaj pierwsze słówko na karcie „Dodaj”.")
                                errorText
                            }
                        }
                    } else {
                        ContentUnavailableView {
                            Label("Nic nie znaleziono.", systemImage: "magnifyingglass")
                        } description: {
                            errorText
                        }
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

    @ViewBuilder private var errorText: some View {
        if let error {
            Text(error).foregroundStyle(.red)
        }
    }

    private func load() async {
        failed = false
        do {
            try await cards.load(query: query)
            error = nil
        } catch {
            // The view went away, or the group ID or the search text changed: the load did not fail.
            guard !Task.isCancelled else { return }
            if cards.cards == nil {
                failed = true
            } else {
                self.error = errorMessage(error)
            }
        }
    }
}

/// The text for a user with no group.
struct NoGroupView: View {
    var body: some View {
        ContentUnavailableView("Nie należysz do żadnej grupy.", systemImage: "person.3", description: Text("Załóż grupę na karcie „Grupa” lub otwórz link z zaproszenia."))
    }
}
