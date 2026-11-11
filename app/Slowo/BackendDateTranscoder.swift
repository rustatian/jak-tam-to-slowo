import Foundation
import OpenAPIRuntime

/// Reads the RFC 3339 date-times of the backend. Go writes a fraction of a second only when the time has one,
/// and the runtime's default transcoder accepts no fraction.
struct BackendDateTranscoder: DateTranscoder {
    private static let fractional = Date.ISO8601FormatStyle(includingFractionalSeconds: true)
    private static let whole = Date.ISO8601FormatStyle()

    func encode(_ date: Date) throws -> String {
        date.formatted(Self.fractional)
    }

    func decode(_ string: String) throws -> Date {
        if let date = try? Self.fractional.parse(string) {
            return date
        }
        return try Self.whole.parse(string)
    }
}

extension Configuration {
    static let backend = Configuration(dateTranscoder: BackendDateTranscoder())
}
