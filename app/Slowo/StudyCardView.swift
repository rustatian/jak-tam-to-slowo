import SwiftUI

/// The strip of the Nauka tab: one capsule for each segment, then the number of due cards with no segment.
struct QueueStripView: View {
    let strip: QueueStrip

    var body: some View {
        HStack(spacing: 4) {
            ForEach(Array(strip.segments.enumerated()), id: \.offset) { _, segment in
                Capsule().fill(color(segment)).frame(height: 10)
            }
            if strip.more > 0 {
                Text(verbatim: "+\(strip.more)").font(.caption.bold()).foregroundStyle(.secondary)
            }
        }
        // The count in the top bar gives the same fact.
        .accessibilityHidden(true)
    }

    private func color(_ segment: QueueStrip.Segment) -> Color {
        switch segment {
        case .current: Theme.accent
        case .good: Theme.good
        case .almost: Theme.almost
        case .miss: Theme.miss
        case .heavy: Theme.heavy
        case .waiting: Theme.waiting
        }
    }
}

/// The card of the Nauka tab: the prompt before the grade, the answer after it. The edges of the next cards show under it.
struct StudyCardView: View {
    let card: Components.Schemas.StudyCard
    let direction: Components.Schemas.Direction
    let review: Components.Schemas.Review?
    /// The text that the learner sent. The gap of a grammar card shows it.
    let answer: String
    /// The number of card edges under the card: 0, 1 or 2.
    let edges: Int
    let next: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                switch review?.grade {
                case .correct: gradeLabel("Dobrze", systemImage: "checkmark.circle.fill", color: Theme.good)
                case .almost: gradeLabel("Prawie dobrze", systemImage: "exclamationmark.circle.fill", color: Theme.almost)
                case .again, nil: Chip(card.card.grammar == nil ? "Słówko" : "Gramatyka")
                }
                Spacer()
                if review?.grade == .almost || review?.grade == .again {
                    Label(card.weightText, systemImage: "scalemass")
                        .font(.caption.bold())
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(border.opacity(0.15), in: Capsule())
                }
            }
            content
        }
        .cardStyle(border: border)
        .background {
            ForEach(0..<edges, id: \.self) { edge in
                RoundedRectangle(cornerRadius: 24)
                    .fill(Theme.surface)
                    .strokeBorder(Theme.line, lineWidth: 2)
                    .padding(.horizontal, CGFloat(edge + 1) * 12)
                    .offset(y: CGFloat(edge + 1) * 10)
                    .zIndex(Double(-edge))
            }
        }
        .padding(.bottom, CGFloat(edges) * 10)
    }

    private var border: Color {
        switch review?.grade {
        case nil: Theme.line
        case .correct: Theme.good
        case .almost: Theme.almost
        case .again: Theme.miss
        }
    }

    private func gradeLabel(_ title: LocalizedStringKey, systemImage: String, color: Color) -> some View {
        Label {
            Text(title)
        } icon: {
            Image(systemName: systemImage).foregroundStyle(color)
        }
        .font(.headline)
    }

    @ViewBuilder private var content: some View {
        if let review {
            if card.card.grammar != nil {
                GrammarAnswer(examplePl: card.card.examplePl, answer: answer, review: review)
            } else {
                Text(direction == .ruPl ? card.card.ru : card.card.pl).font(.subheadline).foregroundStyle(.secondary)
                Text(review.correction).font(.title.bold())
                Text(direction == .ruPl ? card.card.examplePl : card.card.exampleRu)
                Text(direction == .ruPl ? card.card.exampleRu : card.card.examplePl).foregroundStyle(.secondary)
                if let explanation = review.explanation {
                    Text(explanation).foregroundStyle(.secondary)
                }
            }
        } else if let grammar = card.card.grammar {
            GrammarPrompt(examplePl: card.card.examplePl, hint: grammar.hint)
        } else {
            switch direction {
            case .ruPl:
                Text(card.card.ru).font(.title.bold())
                Text(card.card.meaningRu).foregroundStyle(.secondary)
                Text(card.card.exampleRu)
            case .plRu:
                Text(card.card.pl).font(.title.bold())
                Text(card.card.examplePl)
            }
        }
    }
}
