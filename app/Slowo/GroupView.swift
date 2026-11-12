import SwiftUI

/// The form to create a group when the user has no group, else the group and its members.
struct GroupView: View {
    let profile: Profile
    let membership: Membership

    @State private var failed = false
    @State private var error: String?
    @State private var confirmLeave = false
    @State private var invites = 0

    var body: some View {
        // The ZStack keeps one identity for all states, so a change of the state does not start the task again.
        ZStack { content }
            .navigationTitle(profile.me?.groupId == nil ? "Grupa" : membership.group?.name ?? "Grupa")
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
                InvitationSection(membership: membership, invited: { invites += 1 }) { id in
                    change { try await membership.cancel(id) }
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
            .refreshable { await load() }
            .task(id: invites) {
                guard invites > 0 else { return }
                // A bounce or a complaint reaches the server a few seconds after the send. This load shows no error.
                try? await Task.sleep(for: .seconds(5))
                guard !Task.isCancelled else { return }
                try? await membership.load()
                // The task starts again each time the list appears; at 0 it does nothing.
                if !Task.isCancelled { invites = 0 }
            }
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

private struct InvitationSection: View {
    let membership: Membership
    let invited: () -> Void
    let cancel: (String) -> Void

    @State private var email = ""
    @State private var working = false
    @State private var error: String?

    private var address: String { email.trimmingCharacters(in: .whitespacesAndNewlines) }

    var body: some View {
        Section {
            ForEach(membership.invitations, id: \.id) { invitation in
                InvitationRow(invitation: invitation) { cancel(invitation.id) }
            }
            TextField("Adres e-mail", text: $email)
                .keyboardType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
            Button("Zaproś") { Task { await invite() } }
                .disabled(working || address.isEmpty)
        } header: {
            Text("Zaproszenia")
        } footer: {
            VStack(alignment: .leading) {
                Text("Wyślemy e-mail z linkiem. Zaproszenie jest ważne przez 7 dni.")
                if let error {
                    Text(error).foregroundStyle(.red)
                }
            }
        }
    }

    private func invite() async {
        working = true
        defer { working = false }
        do {
            try await membership.invite(email: address)
            email = ""
            error = nil
            invited()
        } catch {
            self.error = errorMessage(error)
        }
    }
}

private struct InvitationRow: View {
    let invitation: Components.Schemas.Invitation
    let cancel: () -> Void

    @State private var confirmCancel = false

    var body: some View {
        VStack(alignment: .leading) {
            Text(invitation.email)
            Group {
                if let name = invitation.invitedBy?.displayName, !name.isEmpty {
                    Text("Zaprasza: \(name).")
                }
                Text("Ważne do \(polishDate(invitation.expiresAt)).")
            }
            .font(.footnote)
            .foregroundStyle(.secondary)
            if let failure = invitation.failure {
                let text: LocalizedStringKey = switch failure {
                case .bounce: "E-mail prawdopodobnie nie dotarł."
                case .complaint: "Odbiorca oznaczył e-mail jako spam."
                }
                Label(text, systemImage: "exclamationmark.triangle")
                    .font(.footnote)
                    .foregroundStyle(.red)
            }
        }
        // The swipe button has no role: a destructive role removes the row at once and closes the dialog on it.
        .swipeActions {
            Button("Anuluj") { confirmCancel = true }
                .tint(.red)
        }
        .confirmationDialog("Anulować zaproszenie dla \(invitation.email)?", isPresented: $confirmCancel, titleVisibility: .visible) {
            Button("Anuluj zaproszenie", role: .destructive, action: cancel)
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
                // The swipe button has no role: a destructive role removes the row at once and closes the dialog on it.
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
