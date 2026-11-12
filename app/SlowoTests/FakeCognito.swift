import Foundation
import Synchronization

@testable import Slowo

/// Answers the Cognito requests with a list of steps, and keeps the requests.
/// Request n gets step n, and the last step answers each later request.
final class FakeCognito: Sendable {
    struct Step: Sendable {
        var status = 200
        var answer = "{}"
        var failure: URLError?
    }

    private let steps: [Step]
    private let delay: Duration
    private let sent = Mutex<[URLRequest]>([])

    init(steps: [Step]) {
        self.steps = steps
        delay = .zero
    }

    init(status: Int = 200, answer: String = "{}", failure: URLError? = nil, delay: Duration = .zero) {
        steps = [Step(status: status, answer: answer, failure: failure)]
        self.delay = delay
    }

    var requests: [URLRequest] { sent.withLock { $0 } }

    func send(_ request: URLRequest) async throws -> (Data, URLResponse) {
        let step = sent.withLock {
            $0.append(request)
            return steps[min($0.count, steps.count) - 1]
        }
        if delay > .zero {
            try await Task.sleep(for: delay)
        }
        if let failure = step.failure {
            throw failure
        }
        let response = HTTPURLResponse(url: request.url!, statusCode: step.status, httpVersion: nil, headerFields: nil)!
        return (Data(step.answer.utf8), response)
    }

    func cognito(now: @escaping @Sendable () -> Date) -> Cognito {
        Cognito(url: URL(string: "https://cognito.test/")!, clientID: "client1", send: send, now: now)
    }
}

let start = try! Date("2026-11-11T10:00:00Z", strategy: .iso8601)
