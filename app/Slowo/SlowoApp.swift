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
        api = Client(serverURL: stage.apiURL, transport: URLSessionTransport(), middlewares: [AuthMiddleware(session: session)])
    }

    var body: some Scene {
        WindowGroup {
            if session.isSignedIn {
                SignedInView(api: api)
            } else {
                SignInView(session: session)
            }
        }
    }
}

/// Loads the account, which makes the user row on the first call, then shows the tabs.
struct SignedInView: View {
    let api: Client

    @State private var loaded = false
    @State private var failed = false

    var body: some View {
        if loaded {
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
                    placeholder("Grupa", systemImage: "person.3")
                }
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
            switch try await api.getMe() {
            case .ok:
                loaded = true
            case .unauthorized:
                // AuthMiddleware has signed out, so the sign-in screen replaces this view.
                break
            default:
                failed = true
            }
        } catch {
            failed = true
        }
    }

    private func placeholder(_ title: LocalizedStringKey, systemImage: String) -> some View {
        ContentUnavailableView(title, systemImage: systemImage, description: Text("Wkrótce"))
    }
}
