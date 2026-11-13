import SwiftUI

/// Says what goes to which AI service, and gives the consent.
struct ConsentView: View {
    let profile: Profile

    @State private var working = false
    @State private var error: String?
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    Text("Aplikacja korzysta z AI, gdy dodajesz fiszki, sprawdzasz odpowiedzi i dyktujesz.")
                    Label {
                        Text("Tekst: słowa, zdania i Twoje odpowiedzi wysyłamy do firmy Anthropic w USA. Ten tekst jest przetwarzany poza Unią Europejską.")
                    } icon: {
                        Image(systemName: "text.bubble")
                    }
                    Label {
                        Text("Dźwięk: nagranie dyktowania wysyłamy do Amazon Transcribe we Frankfurcie, który zamienia je na tekst.")
                    } icon: {
                        Image(systemName: "waveform")
                    }
                    Label {
                        Text("Do AI nie wysyłamy Twojego adresu e-mail ani hasła.")
                    } icon: {
                        Image(systemName: "lock")
                    }
                    Text("Zgodę możesz wycofać w każdej chwili w Ustawieniach. Bez zgody aplikacja działa bez AI: w nauce sprawdza tylko dokładne odpowiedzi.")
                        .foregroundStyle(.secondary)
                    if let error {
                        Text(error).foregroundStyle(.red)
                    }
                }
                .padding()
            }
            .navigationTitle("Zgoda na AI")
            .safeAreaInset(edge: .bottom) {
                VStack(spacing: 12) {
                    Button {
                        Task { await agree() }
                    } label: {
                        Text("Zgadzam się").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    .disabled(working)
                    Button("Nie teraz") { dismiss() }
                }
                .padding()
            }
        }
    }

    private func agree() async {
        working = true
        defer { working = false }
        do {
            try await profile.giveConsent()
            dismiss()
        } catch {
            self.error = errorMessage(error)
        }
    }
}
