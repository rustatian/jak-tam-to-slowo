import OpenAPIURLSession
import SwiftUI

@main
struct SlowoApp: App {
    @State private var session: Session
    @State private var link: InvitationLink
    private let api: Client

    init() {
        let stage = Stage.current
        let session = Session(cognito: Cognito(url: stage.cognitoURL, clientID: stage.clientID), store: TokenStore())
        _session = State(initialValue: session)
        let api = Client(serverURL: stage.apiURL, configuration: .backend, transport: URLSessionTransport(), middlewares: [AuthMiddleware(session: session)])
        self.api = api
        _link = State(initialValue: InvitationLink(api: api))
    }

    var body: some Scene {
        WindowGroup {
            // `onOpenURL` sits on one container of both branches, so it stays in place when the branch changes.
            ZStack {
                if session.isSignedIn {
                    SignedInView(api: api, session: session, link: link)
                } else {
                    SignInView(session: session, link: link)
                }
            }
            .onOpenURL { link.open($0) }
        }
    }
}

/// Loads the profile, which makes the user row on the first call, then shows the tabs.
/// After a sign-in without AI consent, it offers the consent screen once.
/// It shows one sheet at a time, and the invitation of a link goes first.
struct SignedInView: View {
    let session: Session
    let link: InvitationLink

    @State private var profile: Profile
    @State private var membership: Membership
    @State private var failed = false
    @State private var offeringConsent = false
    @State private var tab = AppTab.study

    private enum AppTab {
        case study, words, add, contests, group
    }

    private enum SheetItem: Identifiable {
        case invitation, consent
        var id: Self { self }
    }

    init(api: Client, session: Session, link: InvitationLink) {
        self.session = session
        self.link = link
        let profile = Profile(api: api)
        _profile = State(initialValue: profile)
        _membership = State(initialValue: Membership(api: api, profile: profile))
    }

    var body: some View {
        if profile.me != nil {
            TabView(selection: $tab) {
                Tab("Nauka", systemImage: "graduationcap", value: .study) {
                    placeholder("Nauka", systemImage: "graduationcap")
                }
                Tab("Słówka", systemImage: "books.vertical", value: .words) {
                    placeholder("Słówka", systemImage: "books.vertical")
                }
                Tab("Dodaj", systemImage: "plus.circle", value: .add) {
                    placeholder("Dodaj", systemImage: "plus.circle")
                }
                Tab("Zawody", systemImage: "flag.checkered", value: .contests) {
                    placeholder("Zawody", systemImage: "flag.checkered")
                }
                Tab("Grupa", systemImage: "person.3", value: .group) {
                    NavigationStack {
                        GroupView(profile: profile, membership: membership)
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
            .sheet(item: sheet) { item in
                switch item {
                case .invitation:
                    InvitationView(link: link, profile: profile) { tab = .group }
                case .consent:
                    ConsentView(profile: profile)
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
            try await profile.load()
            offeringConsent = session.signedInThisRun && profile.me?.aiConsentAt == nil
            if link.token != nil {
                link.isPresented = true
            }
        } catch {
            // After a 401 AuthMiddleware has signed out, and the sign-in screen replaces this view.
            failed = true
        }
    }

    /// SwiftUI calls the setter when a swipe or `dismiss()` closes the sheet that shows.
    private var sheet: Binding<SheetItem?> {
        Binding {
            if link.isPresented { .invitation } else if offeringConsent { .consent } else { nil }
        } set: { _ in
            if link.isPresented {
                link.forget()
            } else {
                offeringConsent = false
            }
        }
    }

    private func placeholder(_ title: LocalizedStringKey, systemImage: String) -> some View {
        ContentUnavailableView(title, systemImage: systemImage, description: Text("Wkrótce"))
    }
}
