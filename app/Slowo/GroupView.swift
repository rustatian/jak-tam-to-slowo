import SwiftUI

/// The form to create a group when the user has no group, else the group and its members.
struct GroupView: View {
    let profile: Profile
    let membership: Membership

    @State private var failed = false
    @State private var error: String?
    @State private var confirmLeave = false

    var body: some View {
        // The ZStack keeps one identity for all states, so a change of the state does not start the task again.
        ZStack { content }
            .task(id: profile.me?.groupId) { await load() }
    }

    @ViewBuilder private var content: some View {
        if profile.me?.groupId == nil {
            CreateGroupView(membership: membership)
        } else if let group = membership.group {
            List {
                Section {
                    NavigationLink {
                        NameView(title: "Nazwa grupy", name: group.name) { try await membership.rename($0) }
                    } label: {
                        LabeledContent("Nazwa grupy", value: group.name)
                    }
                }
                Section("Członkowie") {
                    ForEach(membership.members, id: \.id) { member in
                        MemberRow(member: member, isMe: member.id == profile.me?.id) {
                            change { try await membership.remove(member.id) }
                        }
                    }
                }
                Section {
                    Button("Opuść grupę", role: .destructive) { confirmLeave = true }
                        .confirmationDialog("Opuścić grupę?", isPresented: $confirmLeave, titleVisibility: .visible) {
                            Button("Opuść grupę", role: .destructive) {
                                change { try await membership.leave() }
                            }
                        } message: {
                            if membership.members.count == 1 {
                                Text("Jesteś jedyną osobą w grupie, więc grupa zostanie usunięta.")
                            }
                        }
                } footer: {
                    if let error {
                        Text(error).foregroundStyle(.red)
                    }
                }
            }
            .navigationTitle(group.name)
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

    private func load() async {
        failed = false
        do {
            try await membership.load()
            error = nil
        } catch {
            // The view went away or the group ID changed: the load did not fail.
            guard !Task.isCancelled else { return }
            if membership.group == nil {
                failed = true
            } else {
                self.error = errorMessage(error)
            }
        }
    }

    private func change(_ action: @escaping () async throws -> Void) {
        Task {
            do {
                try await action()
                error = nil
            } catch {
                self.error = errorMessage(error)
            }
        }
    }
}

private struct CreateGroupView: View {
    let membership: Membership

    @State private var name = ""
    @State private var working = false
    @State private var error: String?

    var body: some View {
        Form {
            Section {
                TextField("Nazwa grupy", text: $name)
                    .textInputAutocapitalization(.words)
            } header: {
                Text("Nie należysz do żadnej grupy.")
            } footer: {
                VStack(alignment: .leading) {
                    Text("Od 1 do 40 znaków.")
                    if let error {
                        Text(error).foregroundStyle(.red)
                    }
                }
            }
            Section {
                Button("Załóż grupę") { Task { await create() } }
                    .disabled(working || trimmedName(name) == nil)
            }
        }
    }

    private func create() async {
        guard let valid = trimmedName(name) else { return }
        working = true
        defer { working = false }
        do {
            try await membership.create(name: valid)
        } catch {
            self.error = errorMessage(error)
        }
    }
}

private struct MemberRow: View {
    let member: Components.Schemas.Member
    let isMe: Bool
    let remove: () -> Void

    @State private var confirmRemove = false

    var body: some View {
        let name = member.displayName.isEmpty ? "Bez nazwy" : member.displayName
        if isMe {
            LabeledContent(name, value: "Ty")
        } else {
            Text(name)
                // A destructive role removes the row at once and closes the dialog on it.
                .swipeActions {
                    Button("Usuń") { confirmRemove = true }
                        .tint(.red)
                }
                .confirmationDialog("Usunąć osobę „\(name)” z grupy?", isPresented: $confirmRemove, titleVisibility: .visible) {
                    Button("Usuń", role: .destructive, action: remove)
                }
        }
    }
}
