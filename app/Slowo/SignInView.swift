import SwiftUI

struct SignInView: View {
    let session: Session

    @State private var email = ""
    @State private var password = ""
    @State private var error: String?
    @State private var working = false
    @State private var resetting = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("E-mail", text: $email)
                        .textContentType(.username)
                        .keyboardType(.emailAddress)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    SecureField("Hasło", text: $password)
                        .textContentType(.password)
                        .onSubmit { Task { await signIn() } }
                } footer: {
                    if let error {
                        Text(error).foregroundStyle(.red)
                    }
                }
                Section {
                    Button("Zaloguj się") { Task { await signIn() } }
                        .disabled(working || email.isEmpty || password.isEmpty)
                    Button("Nie pamiętasz hasła?") { resetting = true }
                }
            }
            .navigationTitle("Jak tam to słowo?")
            .sheet(isPresented: $resetting) {
                PasswordResetView(session: session, email: email)
            }
        }
    }

    private func signIn() async {
        working = true
        defer { working = false }
        do {
            try await session.signIn(email: email, password: password)
        } catch {
            self.error = errorMessage(error)
        }
    }
}
