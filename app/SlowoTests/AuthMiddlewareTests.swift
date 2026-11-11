import Foundation
import HTTPTypes
import Synchronization
import Testing

@testable import Slowo

struct AuthMiddlewareCase: Sendable, CustomTestStringConvertible {
    let name: String
    let status: HTTPResponse.Status
    let signedIn: Bool

    var testDescription: String { name }
}

@MainActor
@Test(arguments: [
    AuthMiddlewareCase(name: "adds the bearer token", status: .ok, signedIn: true),
    AuthMiddlewareCase(name: "a 401 signs out", status: .unauthorized, signedIn: false),
])
func authMiddleware(_ c: AuthMiddlewareCase) async throws {
    let store = TokenStore(service: "test." + UUID().uuidString)
    defer { try? store.delete() }
    try store.save(Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(3600)))
    let session = Session(cognito: FakeCognito().cognito(now: { start }), store: store, now: { start })
    let sent = Mutex<String?>(nil)

    let (response, _) = try await AuthMiddleware(session: session).intercept(
        HTTPRequest(method: .get, scheme: "https", authority: "api.test", path: "/me"),
        body: nil,
        baseURL: URL(string: "https://api.test")!,
        operationID: "getMe",
        next: { request, _, _ in
            sent.withLock { $0 = request.headerFields[.authorization] }
            return (HTTPResponse(status: c.status), nil)
        }
    )

    #expect(sent.withLock { $0 } == "Bearer a1")
    #expect(response.status == c.status)
    #expect(session.isSignedIn == c.signedIn)
}
