import OpenAPIURLSession
import SwiftUI
import UserNotifications

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
                    // The sign-in screens keep the system look. The sheets of the signed-in screens take the tint and the font from here.
                    SignedInView(api: api, session: session, link: link)
                        .tint(Theme.accent)
                        .fontDesign(.rounded)
                        .groundBackground()
                } else {
                    SignInView(session: session, link: link)
                }
            }
            .onOpenURL { link.open($0) }
            // Every sign-out path ends in `Session.signOut()`, so this removes the notification for all of them.
            .onChange(of: session.isSignedIn) { _, signedIn in
                if !signedIn {
                    UNUserNotificationCenter.current().removePendingNotificationRequests(withIdentifiers: [Reminder.id])
                }
            }
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
    @State private var cards: Cards
    @State private var study: Study
    @State private var reminder: Reminder
    @State private var draft: CardDraft
    @State private var check: GrammarCheck
    @State private var failed = false
    @State private var offeringConsent = false
    @State private var tab = AppTab.study

    private enum AppTab {
        case study, words, add, contests, group
    }

    init(api: Client, session: Session, link: InvitationLink) {
        self.session = session
        self.link = link
        let profile = Profile(api: api)
        _profile = State(initialValue: profile)
        _membership = State(initialValue: Membership(api: api, profile: profile))
        // The grammar check creates its cards through the object of the Słówka tab, so the list shows them with no pull.
        let cards = Cards(api: api, profile: profile)
        _cards = State(initialValue: cards)
        let reminder = Reminder(center: UNUserNotificationCenter.current(), profile: profile)
        _reminder = State(initialValue: reminder)
        _study = State(initialValue: Study(api: api, profile: profile, reminder: reminder))
        _draft = State(initialValue: CardDraft(api: api, profile: profile))
        _check = State(initialValue: GrammarCheck(api: api, profile: profile, collection: cards))
    }

    var body: some View {
        if profile.me != nil {
            TabView(selection: $tab) {
                Tab("Nauka", systemImage: "graduationcap", value: .study) {
                    NavigationStack {
                        StudyView(profile: profile, study: study)
                    }
                }
                Tab("Słówka", systemImage: "books.vertical", value: .words) {
                    NavigationStack {
                        WordsView(profile: profile, cards: cards)
                    }
                }
                Tab("Dodaj", systemImage: "plus.circle", value: .add) {
                    NavigationStack {
                        AddView(profile: profile, cards: cards, draft: draft, check: check) { offeringConsent = true }
                    }
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
                                        SettingsView(profile: profile, session: session, reminder: reminder)
                                    } label: {
                                        Label("Ustawienia", systemImage: "gearshape")
                                    }
                                }
                            }
                    }
                }
            }
            // The keyboard covers the tab bar, so a drag on a screen hides the keyboard.
            .scrollDismissesKeyboard(.immediately)
            // A new group has its own due time. The next queue answer schedules the notification again.
            .onChange(of: profile.me?.groupId) { reminder.clear() }
            // SwiftUI calls each setter with false also when the code closes the sheet.
            // The invitation setter forgets the token only while the model still shows the invitation.
            // The consent setter ends the offer only when no invitation takes the place of the consent sheet.
            .sheet(isPresented: Binding(get: { link.isPresented }, set: { if !$0 && link.isPresented { link.forget() } })) {
                InvitationView(link: link, profile: profile) { tab = .group }
            }
            .sheet(isPresented: Binding(get: { offeringConsent && !link.isPresented }, set: { if !$0 && !link.isPresented { offeringConsent = false } })) {
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
            if link.token != nil {
                link.isPresented = true
            }
        } catch {
            // After a 401 AuthMiddleware has signed out, and the sign-in screen replaces this view.
            failed = true
        }
    }

    private func placeholder(_ title: LocalizedStringKey, systemImage: String) -> some View {
        ContentUnavailableView(title, systemImage: systemImage, description: Text("Wkrótce")).groundBackground()
    }
}
