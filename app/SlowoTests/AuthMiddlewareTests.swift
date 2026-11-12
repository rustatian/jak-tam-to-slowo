import Foundation
import HTTPTypes
import Synchronization
import Testing

@testable import Slowo

struct AuthMiddlewareCase: Sendable, CustomTestStringConvertible {
    let name: String
    let operationID: String
    /// The store has tokens before the call.
    let stored: Bool
    let status: HTTPResponse.Status
    /// The Authorization header that the server gets.
    let authorization: String?
    let signedIn: Bool

    var testDescription: String { name }
}

@MainActor
@Test(arguments: [
    AuthMiddlewareCase(name: "adds the bearer token", operationID: "getMe", stored: true, status: .ok, authorization: "Bearer a1", signedIn: true),
    AuthMiddlewareCase(name: "a 401 signs out", operationID: "getMe", stored: true, status: .unauthorized, authorization: "Bearer a1", signedIn: false),
    AuthMiddlewareCase(
        name: "the preview of a signed-out person has no token", operationID: "previewInvitation", stored: false, status: .ok, authorization: nil, signedIn: false
    ),
    AuthMiddlewareCase(
        name: "the preview of a signed-in person has no token", operationID: "previewInvitation", stored: true, status: .ok, authorization: nil, signedIn: true
    ),
])
func authMiddleware(_ c: AuthMiddlewareCase) async throws {
    let store = TokenStore(service: "test." + UUID().uuidString)
    defer { try? store.delete() }
    if c.stored {
        try store.save(Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(3600)))
    }
    let session = Session(cognito: FakeCognito().cognito(now: { start }), store: store, now: { start })
    let sent = Mutex<String?>(nil)

    let (response, _) = try await AuthMiddleware(session: session).intercept(
        HTTPRequest(method: .get, scheme: "https", authority: "api.test", path: "/me"),
        body: nil,
        baseURL: URL(string: "https://api.test")!,
        operationID: c.operationID,
        next: { request, _, _ in
            sent.withLock { $0 = request.headerFields[.authorization] }
            return (HTTPResponse(status: c.status), nil)
        }
    )

    #expect(sent.withLock { $0 } == c.authorization)
    #expect(response.status == c.status)
    #expect(session.isSignedIn == c.signedIn)
}
