import SwiftUI

/// Sends a code to the email, then sets the new password with it and signs in.
struct PasswordResetView: View {
    let session: Session
    @State var email: String

    @State private var code = ""
    @State private var newPassword = ""
    @State private var codeSent = false
    @State private var error: String?
    @State private var working = false
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Form {
                if codeSent {
                    Section {
                        TextField("Kod z e-maila", text: $code)
                            .textContentType(.oneTimeCode)
                            .keyboardType(.numberPad)
                        SecureField("Nowe hasło", text: $newPassword)
                            .textContentType(.newPassword)
                    } footer: {
                        VStack(alignment: .leading) {
                            Text(passwordRules)
                            if let error {
                                Text(error).foregroundStyle(.red)
                            }
                        }
                    }
                    Button("Ustaw hasło") {
                        run {
                            try await session.resetPassword(email: email, code: code, newPassword: newPassword)
                            dismiss()
                        }
                    }
                    .disabled(working || code.isEmpty || newPassword.isEmpty)
                } else {
                    Section {
                        TextField("E-mail", text: $email)
                            .textContentType(.username)
                            .keyboardType(.emailAddress)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                    } header: {
                        Text("Wyślemy kod na Twój e-mail.")
                    } footer: {
                        if let error {
                            Text(error).foregroundStyle(.red)
                        }
                    }
                    Button("Wyślij kod") {
                        run {
                            try await session.sendResetCode(email: email)
                            codeSent = true
                        }
                    }
                    .disabled(working || email.isEmpty)
                }
            }
            .navigationTitle("Nowe hasło")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Anuluj") { dismiss() }
                }
            }
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
