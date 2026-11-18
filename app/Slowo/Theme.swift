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
    /// The border of a card and of a field.
    static let line = Color(light: 0xDDE3EE, dark: 0x3A4666)
    /// The colors of a grade and of the strip. They are for borders, fills and symbols. Text stays primary or secondary.
    static let good = Color(light: 0x2DB36A, dark: 0x4CC98A)
    static let almost = Color(light: 0xE8A317, dark: 0xF0B94A)
    static let miss = Color(light: 0xD93A3A, dark: 0xFF7A7A)
    /// A due card in the strip.
    static let waiting = Color(light: 0xC3CCDD, dark: 0x46527A)
    /// A due card in the strip with more misses than correct answers.
    static let heavy = Color(light: 0xE07A7A, dark: 0xC26472)
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
                .background { shape.fill(Theme.accent.mix(with: .black, by: 0.3)).offset(y: edge) }
                .padding(.bottom, edge)
                // One layer, so the label keeps its contrast on the face when the button is not active.
                .compositingGroup()
                .opacity(isEnabled ? 1 : 0.45)
                .contentShape(shape)
        }
    }
}

/// A short label in a capsule, for the kind of a card.
struct Chip: View {
    let text: LocalizedStringKey

    init(_ text: LocalizedStringKey) {
        self.text = text
    }

    var body: some View {
        // The text is primary: the accent on this capsule has a contrast below 4.5:1.
        Text(text)
            .font(.caption.bold())
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
            .background(Theme.accent.opacity(0.1), in: Capsule())
    }
}

extension View {
    /// Fills the available space and puts the ground color behind the view, also under the safe areas.
    func groundBackground() -> some View {
        frame(maxWidth: .infinity, maxHeight: .infinity).background(Theme.ground)
    }

    /// A card: the surface color with a border and a darker lower edge.
    func cardStyle(border: Color = Theme.line) -> some View {
        let shape = RoundedRectangle(cornerRadius: 24)
        let edge: CGFloat = 6
        return padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Theme.surface, in: shape)
            .overlay { shape.strokeBorder(border, lineWidth: 2) }
            .background { shape.fill(border.mix(with: .black, by: 0.2)).offset(y: edge) }
            .padding(.bottom, edge)
    }

    /// For a button that is the only row of its section: the primary style, with no row background and no row insets.
    func primaryButtonRow() -> some View {
        buttonStyle(PrimaryButtonStyle())
            .listRowInsets(EdgeInsets())
            .listRowBackground(Color.clear)
    }
}
