import SwiftUI

/// The invitation of the link that the app opened. A nil profile means that nobody is signed in.
struct InvitationView: View {
    let link: InvitationLink
    let profile: Profile?
    let accepted: () -> Void

    @State private var loadError: (any Error)?
    @State private var groupName = ""
    @State private var displayName = ""
    @State private var working = false
    @State private var error: String?
    @State private var confirmLeave = false

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Zaproszenie")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Zamknij") { link.forget() }
                    }
                }
        }
        .task(id: link.token) { await load() }
    }

    @ViewBuilder private var content: some View {
        if let loadError {
            ContentUnavailableView {
                Label(errorMessage(loadError), systemImage: "exclamationmark.triangle")
            } actions: {
                // A retry at once cannot help a dead invitation or the rate limit.
                switch loadError as? InvitationError {
                case .notFound, .tooManyTries:
                    EmptyView()
                default:
                    Button("Spróbuj ponownie") { Task { await load() } }
                }
            }
        } else if let preview = link.preview {
            Form {
                Section {
                    line(preview)
                }
                if let profile {
                    acceptSections(profile, newGroup: preview.groupName == nil)
                } else {
                    Section {
                        Button("Zaloguj się") { link.isPresented = false }
                    }
                }
            }
        } else {
            ProgressView()
        }
    }

    /// Verbatim text, so a name with Markdown characters shows as typed.
    private func line(_ preview: Components.Schemas.InvitationPreview) -> Text {
        guard let name = preview.groupName else {
            return Text("Masz zaproszenie do założenia nowej grupy.")
        }
        if let inviter = preview.invitedBy {
            return Text(verbatim: "\(inviter) zaprasza Cię do grupy „\(name)”.")
        }
        return Text(verbatim: "Masz zaproszenie do grupy „\(name)”.")
    }

    @ViewBuilder private func acceptSections(_ profile: Profile, newGroup: Bool) -> some View {
        let needsDisplayName = profile.me?.displayName.isEmpty == true
        if newGroup {
            Section {
                TextField("Nazwa grupy", text: $groupName)
                    .textInputAutocapitalization(.words)
            } footer: {
                Text("Od 1 do 40 znaków.")
            }
        }
        if needsDisplayName {
            Section {
                TextField("Twoja nazwa w grupie", text: $displayName)
                    .textInputAutocapitalization(.words)
            } footer: {
                Text("Od 1 do 40 znaków.")
            }
        }
        Section {
            Button(newGroup ? "Załóż grupę" : "Dołącz do grupy") {
                Task { await accept(profile, newGroup: newGroup) }
            }
            .disabled(working || (newGroup && trimmedName(groupName) == nil) || (needsDisplayName && trimmedName(displayName) == nil))
            .confirmationDialog("Opuścić obecną grupę?", isPresented: $confirmLeave, titleVisibility: .visible) {
                Button(newGroup ? "Opuść i załóż grupę" : "Opuść i dołącz", role: .destructive) {
                    Task { await accept(profile, newGroup: newGroup, leaveGroup: true) }
                }
            } message: {
                if newGroup {
                    Text("Gdy założysz nową grupę, opuścisz obecną. Jeśli jesteś w niej jedyną osobą, grupa zostanie usunięta.")
                } else {
                    Text("Gdy dołączysz, opuścisz obecną grupę. Jeśli jesteś w niej jedyną osobą, grupa zostanie usunięta.")
                }
            }
        } footer: {
            if let error {
                Text(error).foregroundStyle(.red)
            }
        }
    }

    private func load() async {
        // A forget clears the token while the sheet closes: the closing sheet keeps its content.
        guard link.token != nil else { return }
        loadError = nil
        error = nil
        do {
            try await link.loadPreview()
        } catch {
            // The sheet closed or the token changed: the load did not fail.
            guard !Task.isCancelled else { return }
            loadError = error
        }
    }

    private func accept(_ profile: Profile, newGroup: Bool, leaveGroup: Bool = false) async {
        working = true
        defer { working = false }
        do {
            if profile.me?.displayName.isEmpty == true, let name = trimmedName(displayName) {
                try await profile.update(.init(displayName: name))
            }
            try await link.accept(profile: profile, groupName: newGroup ? trimmedName(groupName) : nil, leaveGroup: leaveGroup)
            accepted()
        } catch InvitationError.inAnotherGroup {
            error = nil
            confirmLeave = true
        } catch {
            self.error = errorMessage(error)
        }
    }
}
