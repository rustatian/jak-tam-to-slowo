import SwiftUI

struct SettingsView: View {
    let profile: Profile
    let session: Session

    @State private var pendingDirection: Components.Schemas.Direction?
    @State private var pendingReminder: Bool?
    @State private var error: String?
    @State private var givingConsent = false
    @State private var confirmWithdraw = false
    @State private var confirmLogOut = false

    var body: some View {
        Form {
            if let me = profile.me {
                Section("Profil") {
                    NavigationLink {
                        NameView(title: "Nazwa w grupie", name: me.displayName) { try await profile.update(.init(displayName: $0)) }
                    } label: {
                        LabeledContent("Nazwa w grupie", value: me.displayName.isEmpty ? "Brak" : me.displayName)
                    }
                    Picker("Kierunek nauki", selection: direction(me)) {
                        Text("Z rosyjskiego na polski").tag(Components.Schemas.Direction.ruPl)
                        Text("Z polskiego na rosyjski").tag(Components.Schemas.Direction.plRu)
                    }
                    Toggle("Przypomnienie o powtórkach", isOn: reminder(me))
                }
                Section("AI") {
                    LabeledContent("Zgoda na AI", value: me.aiConsentAt.map { "od " + polishDate($0) } ?? "brak")
                    if me.aiConsentAt == nil {
                        Button("Udziel zgody") { givingConsent = true }
                    } else {
                        Button("Wycofaj zgodę", role: .destructive) { confirmWithdraw = true }
                            .confirmationDialog("Wycofać zgodę na AI?", isPresented: $confirmWithdraw, titleVisibility: .visible) {
                                Button("Wycofaj", role: .destructive) {
                                    save { try await profile.withdrawConsent() }
                                }
                            }
                    }
                }
                Section("Konto") {
                    LabeledContent("E-mail", value: me.email)
                    Button("Wyloguj się", role: .destructive) { confirmLogOut = true }
                        .confirmationDialog("Wylogować się?", isPresented: $confirmLogOut, titleVisibility: .visible) {
                            Button("Wyloguj się", role: .destructive) {
                                Task { await session.logOut() }
                            }
                        }
                }
                if let error {
                    Section {
                        Text(error).foregroundStyle(.red)
                    }
                }
            }
        }
        .navigationTitle("Ustawienia")
        .sheet(isPresented: $givingConsent) {
            ConsentView(profile: profile)
        }
    }

    /// Shows the chosen value at once; after a failure it shows the stored value again.
    private func direction(_ me: Components.Schemas.Me) -> Binding<Components.Schemas.Direction> {
        Binding {
            pendingDirection ?? me.direction
        } set: { value in
            pendingDirection = value
            save(then: { pendingDirection = nil }) { try await profile.update(.init(direction: value)) }
        }
    }

    private func reminder(_ me: Components.Schemas.Me) -> Binding<Bool> {
        Binding {
            pendingReminder ?? me.reminder
        } set: { value in
            pendingReminder = value
            save(then: { pendingReminder = nil }) { try await profile.update(.init(reminder: value)) }
        }
    }

    private func save(then done: @escaping () -> Void = {}, _ action: @escaping () async throws -> Void) {
        Task {
            do {
                try await action()
                error = nil
            } catch {
                self.error = errorMessage(error)
            }
            done()
        }
    }
}

/// The interface is Polish only, and `Locale.current` follows the language of the device.
func polishDate(_ date: Date) -> String {
    date.formatted(.dateTime.day().month(.wide).year().locale(Locale(identifier: "pl_PL")))
}

struct NameView: View {
    let title: LocalizedStringKey
    let save: (String) async throws -> Void

    @State private var name: String
    @State private var working = false
    @State private var error: String?
    @Environment(\.dismiss) private var dismiss

    init(title: LocalizedStringKey, name: String, save: @escaping (String) async throws -> Void) {
        self.title = title
        self.save = save
        _name = State(initialValue: name)
    }

    var body: some View {
        Form {
            Section {
                TextField(title, text: $name)
                    .textInputAutocapitalization(.words)
            } footer: {
                VStack(alignment: .leading) {
                    Text("Od 1 do 40 znaków.")
                    if let error {
                        Text(error).foregroundStyle(.red)
                    }
                }
            }
        }
        .navigationTitle(title)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button("Zapisz") { Task { await submit() } }
                    .disabled(working || trimmedName(name) == nil)
            }
        }
    }

    private func submit() async {
        guard let valid = trimmedName(name) else { return }
        working = true
        defer { working = false }
        do {
            try await save(valid)
            dismiss()
        } catch {
            self.error = errorMessage(error)
        }
    }
}
