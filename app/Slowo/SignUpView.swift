import SwiftUI

/// Confirms an account with the code from the email, then signs in.
/// For an invitation, it first makes the account with the email and the password.
struct SignUpView: View {
    enum Start: Hashable, Identifiable {
        /// The token of the invitation. The sheet starts at the form.
        case invitation(String)
        /// An account with no confirmation. The sheet starts at the code step.
        case unconfirmed(email: String, password: String)

        var id: Self { self }
    }

    let session: Session
    let start: Start

    @State private var email = ""
    @State private var password = ""
    @State private var code = ""
    @State private var codeSent = false
    @State private var newCode = false
    @State private var error: String?
    @State private var working = false
    @Environment(\.dismiss) private var dismiss

    init(session: Session, start: Start) {
        self.session = session
        self.start = start
        if case let .unconfirmed(email, password) = start {
            _email = State(initialValue: email)
            _password = State(initialValue: password)
            _codeSent = State(initialValue: true)
        }
    }

    var body: some View {
        NavigationStack {
            Form {
                if codeSent {
                    Section {
                        TextField("Kod z e-maila", text: $code)
                            .textContentType(.oneTimeCode)
                            .keyboardType(.numberPad)
                    } header: {
                        // Verbatim text, so an address with Markdown characters shows as typed.
                        Text(verbatim: "Wysłaliśmy kod na adres \(email.trimmingCharacters(in: .whitespacesAndNewlines)).")
                    } footer: {
                        if let error {
                            Text(error).foregroundStyle(.red)
                        } else if newCode {
                            Text("Wysłaliśmy nowy kod. Wpisz kod z najnowszego e-maila.")
                        }
                    }
                    Button("Potwierdź") {
                        run { try await session.confirmSignUp(email: email, code: code, password: password) }
                    }
                    .disabled(working || code.isEmpty)
                    Button("Wyślij nowy kod", action: sendCode)
                        .disabled(working)
                } else if case let .invitation(token) = start {
                    Section {
                        TextField("E-mail", text: $email)
                            .textContentType(.username)
                            .keyboardType(.emailAddress)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                        SecureField("Hasło", text: $password)
                            .textContentType(.newPassword)
                    } footer: {
                        VStack(alignment: .leading) {
                            Text(passwordRules)
                            if let error {
                                Text(error).foregroundStyle(.red)
                            }
                        }
                    }
                    Button("Załóż konto") {
                        run {
                            try await session.signUp(email: email, password: password, invitation: token)
                            codeSent = true
                        }
                    }
                    .disabled(working || email.isEmpty || password.isEmpty)
                }
            }
            .navigationTitle("Nowe konto")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Anuluj") { dismiss() }
                }
            }
        }
        .task {
            if case .unconfirmed = start { sendCode() }
        }
    }

    private func sendCode() {
        run {
            try await session.sendSignUpCode(email: email)
            newCode = true
        }
    }

    private func run(_ action: @escaping () async throws -> Void) {
        Task {
            working = true
            defer { working = false }
            do {
                try await action()
                error = nil
            } catch {
                self.error = errorMessage(error)
            }
        }
    }
}
