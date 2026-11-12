import SwiftUI

struct SignInView: View {
    let session: Session
    let link: InvitationLink

    @State private var email = ""
    @State private var password = ""
    @State private var error: String?
    @State private var working = false
    @State private var resetting = false
    @State private var signUp: SignUpView.Start?

    var body: some View {
        ScrollView {
            VStack(spacing: 28) {
                letters
                Text("Jak tam to słowo?")
                    .font(.largeTitle.bold())
                    .multilineTextAlignment(.center)
                VStack(spacing: 0) {
                    TextField("E-mail", text: $email)
                        .textContentType(.username)
                        .keyboardType(.emailAddress)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .padding()
                    Divider().padding(.leading)
                    SecureField("Hasło", text: $password)
                        .textContentType(.password)
                        .onSubmit { Task { await signIn() } }
                        .padding()
                }
                .glassEffect(in: .rect(cornerRadius: 22))
                if let error {
                    Text(error)
                        .font(.footnote)
                        .foregroundStyle(.red)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal)
                }
                VStack(spacing: 16) {
                    Button { Task { await signIn() } } label: {
                        Text("Zaloguj się").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.glassProminent)
                    .controlSize(.large)
                    .disabled(working || email.isEmpty || password.isEmpty)
                    Button("Nie pamiętasz hasła?") { resetting = true }
                }
            }
            .padding(24)
        }
        .defaultScrollAnchor(.center)
        .scrollBounceBehavior(.basedOnSize)
        .scrollDismissesKeyboard(.interactively)
        .background { backdrop }
        .sheet(isPresented: $resetting) {
            PasswordResetView(session: session, email: email)
        }
        // SwiftUI calls the setter with false for each close, also after "Zaloguj się" and "Załóż konto".
        // The setter forgets the token only while the model still shows the sheet, so both buttons keep it.
        .sheet(isPresented: Binding(get: { link.isPresented }, set: { if !$0 && link.isPresented { link.forget() } })) {
            InvitationView(link: link, profile: nil, accepted: {}, signUp: { if let token = link.token { signUp = .invitation(token) }; link.isPresented = false })
        }
        // The sign-up sheet shows only while the invitation sheet does not show.
        // "Anuluj" and a swipe end the sign-up through the setter.
        // An invitation link ends it through `onChange`, because `sheet(item:)` does not call this setter when the getter closes the sheet.
        .sheet(item: Binding(get: { link.isPresented ? nil : signUp }, set: { if $0 == nil { signUp = nil } })) { start in
            SignUpView(session: session, start: start)
        }
        .onChange(of: link.isPresented) { _, shown in if shown { signUp = nil } }
    }

    /// The two letters of the app icon: the Russian Ж and the Polish Ł.
    private var letters: some View {
        HStack(spacing: 12) {
            letter("Ж").offset(y: -8)
            letter("Ł").offset(y: 8)
        }
        .accessibilityHidden(true)
    }

    private func letter(_ text: String) -> some View {
        Text(text)
            .font(.system(size: 44, weight: .bold, design: .rounded))
            .foregroundStyle(.tint)
            .frame(width: 84, height: 84)
            .glassEffect(in: .rect(cornerRadius: 24))
    }

    /// System colors over the system background, so the backdrop follows the light and the dark appearance.
    private var backdrop: some View {
        MeshGradient(
            width: 3, height: 3,
            points: [
                [0, 0], [0.5, 0], [1, 0],
                [0, 0.5], [0.65, 0.4], [1, 0.5],
                [0, 1], [0.5, 1], [1, 1],
            ],
            colors: [
                .blue.opacity(0.45), .indigo.opacity(0.30), .cyan.opacity(0.30),
                .cyan.opacity(0.18), .blue.opacity(0.22), .indigo.opacity(0.18),
                .blue.opacity(0.04), .cyan.opacity(0.12), .blue.opacity(0.28),
            ]
        )
        .background(.background)
        .ignoresSafeArea()
    }

    private func signIn() async {
        working = true
        defer { working = false }
        do {
            try await session.signIn(email: email, password: password)
        } catch CognitoError.service("UserNotConfirmedException") {
            error = nil
            signUp = .unconfirmed(email: email, password: password)
        } catch {
            self.error = errorMessage(error)
        }
    }
}
