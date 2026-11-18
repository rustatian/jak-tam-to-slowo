import SwiftUI

/// The colors of the screens after sign-in. Each color has a light value and a dark value and follows the appearance of the phone.
enum Theme {
    /// The background of each screen.
    static let ground = Color(light: 0xF3F5FA, dark: 0x1B2233)
    /// List rows, cards and fields.
    static let surface = Color(light: 0xFFFFFF, dark: 0x252E44)
    /// The tint and the primary button.
    static let accent = Color(light: 0x3D5AFE, dark: 0x8C9BFF)
    /// The text on the primary button.
    static let onAccent = Color(light: 0xFFFFFF, dark: 0x121830)
}

private extension Color {
    /// A color from two RGB values in the form 0xRRGGBB.
    init(light: UInt32, dark: UInt32) {
        self.init(uiColor: UIColor { traits in
            let hex = traits.userInterfaceStyle == .dark ? dark : light
            return UIColor(red: CGFloat(hex >> 16 & 0xFF) / 255, green: CGFloat(hex >> 8 & 0xFF) / 255, blue: CGFloat(hex & 0xFF) / 255, alpha: 1)
        })
    }
}

/// A form with the ground color behind it and the surface color behind each row.
struct ThemedForm<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        Form {
            Group { content }.listRowBackground(Theme.surface)
        }
        .scrollContentBackground(.hidden)
        .background(Theme.ground)
    }
}

/// A list with the ground color behind it and the surface color behind each row.
struct ThemedList<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        List {
            Group { content }.listRowBackground(Theme.surface)
        }
        .scrollContentBackground(.hidden)
        .background(Theme.ground)
    }
}

/// The main action of a screen: an accent face on a darker lower edge. A press moves the face down onto the edge.
struct PrimaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        Face(configuration: configuration)
    }

    /// A view, because a button style cannot read `isEnabled` from the environment.
    private struct Face: View {
        let configuration: Configuration
        @Environment(\.isEnabled) private var isEnabled

        var body: some View {
            let shape = RoundedRectangle(cornerRadius: 16)
            let edge: CGFloat = 5
            configuration.label
                .font(.headline)
                .foregroundStyle(Theme.onAccent)
                .padding(.horizontal, 16)
                .padding(.vertical, 8)
                .frame(maxWidth: .infinity, minHeight: 56)
                .background(Theme.accent, in: shape)
                .offset(y: configuration.isPressed ? edge : 0)
                .padding(.bottom, edge)
                .background(Theme.accent.mix(with: .black, by: 0.3), in: shape)
                .opacity(isEnabled ? 1 : 0.45)
                .contentShape(shape)
        }
    }
}

extension View {
    /// Fills the available space and puts the ground color behind the view, also under the safe areas.
    func groundBackground() -> some View {
        frame(maxWidth: .infinity, maxHeight: .infinity).background(Theme.ground)
    }

    /// For a button that is the only row of its section: the primary style, with no row background and no row insets.
    func primaryButtonRow() -> some View {
        buttonStyle(PrimaryButtonStyle())
            .listRowInsets(EdgeInsets())
            .listRowBackground(Color.clear)
    }
}
