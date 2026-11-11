import OpenAPIURLSession
import SwiftUI

@main
struct SlowoApp: App {
    @State private var session: Session
    private let api: Client

    init() {
        let stage = Stage.current
        let session = Session(cognito: Cognito(url: stage.cognitoURL, clientID: stage.clientID), store: TokenStore())
        _session = State(initialValue: session)
        api = Client(serverURL: stage.apiURL, configuration: .backend, transport: URLSessionTransport(), middlewares: [AuthMiddleware(session: session)])
    }

    var body: some Scene {
        WindowGroup {
            if session.isSignedIn {
                SignedInView(api: api, session: session)
            } else {
                SignInView(session: session)
            }
        }
    }
}

/// Loads the profile, which makes the user row on the first call, then shows the tabs.
/// After a sign-in without AI consent, it offers the consent screen once.
struct SignedInView: View {
    let session: Session

    @State private var profile: Profile
    @State private var failed = false
    @State private var offeringConsent = false

    init(api: Client, session: Session) {
        self.session = session
        _profile = State(initialValue: Profile(api: api))
    }

    var body: some View {
        if profile.me != nil {
            TabView {
                Tab("Nauka", systemImage: "graduationcap") {
                    placeholder("Nauka", systemImage: "graduationcap")
                }
                Tab("Słówka", systemImage: "books.vertical") {
                    placeholder("Słówka", systemImage: "books.vertical")
                }
                Tab("Dodaj", systemImage: "plus.circle") {
                    placeholder("Dodaj", systemImage: "plus.circle")
                }
                Tab("Zawody", systemImage: "flag.checkered") {
                    placeholder("Zawody", systemImage: "flag.checkered")
                }
                Tab("Grupa", systemImage: "person.3") {
                    NavigationStack {
                        placeholder("Grupa", systemImage: "person.3")
                            .toolbar {
                                ToolbarItem(placement: .topBarTrailing) {
                                    NavigationLink {
                                        SettingsView(profile: profile, session: session)
                                    } label: {
                                        Label("Ustawienia", systemImage: "gearshape")
                                    }
                                }
                            }
                    }
                }
            }
            .sheet(isPresented: $offeringConsent) {
                ConsentView(profile: profile)
            }
        } else if failed {
            ContentUnavailableView {
                Label("Nie udało się połączyć z serwerem.", systemImage: "wifi.exclamationmark")
            } actions: {
                Button("Spróbuj ponownie") { failed = false }
            }
        } else {
            ProgressView().task { await load() }
        }
    }

    private func load() async {
        do {
            try await profile.load()
            offeringConsent = session.signedInThisRun && profile.me?.aiConsentAt == nil
        } catch {
            // After a 401 AuthMiddleware has signed out, and the sign-in screen replaces this view.
            failed = true
        }
    }

    private func placeholder(_ title: LocalizedStringKey, systemImage: String) -> some View {
        ContentUnavailableView(title, systemImage: systemImage, description: Text("Wkrótce"))
    }
}
