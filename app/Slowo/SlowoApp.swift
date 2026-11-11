import SwiftUI

@main
struct SlowoApp: App {
    var body: some Scene {
        WindowGroup {
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
        }
    }

    private func placeholder(_ title: LocalizedStringKey, systemImage: String) -> some View {
        ContentUnavailableView(title, systemImage: systemImage, description: Text("Wkrótce"))
    }
}
