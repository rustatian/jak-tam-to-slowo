import Foundation
import Synchronization

@testable import Slowo

/// Answers every Cognito request with one fixed answer, or fails it, and keeps the requests.
final class FakeCognito: Sendable {
    private let status: Int
    private let answer: String
    private let failure: URLError?
    private let delay: Duration
    private let sent = Mutex<[URLRequest]>([])

    init(status: Int = 200, answer: String = "{}", failure: URLError? = nil, delay: Duration = .zero) {
        self.status = status
        self.answer = answer
        self.failure = failure
        self.delay = delay
    }

    var requests: [URLRequest] { sent.withLock { $0 } }

    func send(_ request: URLRequest) async throws -> (Data, URLResponse) {
        sent.withLock { $0.append(request) }
        if delay > .zero {
            try await Task.sleep(for: delay)
        }
        if let failure {
            throw failure
        }
        let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!
        return (Data(answer.utf8), response)
    }

    func cognito(now: @escaping @Sendable () -> Date) -> Cognito {
        Cognito(url: URL(string: "https://cognito.test/")!, clientID: "client1", send: send, now: now)
    }
}

let start = try! Date("2026-11-11T10:00:00Z", strategy: .iso8601)
