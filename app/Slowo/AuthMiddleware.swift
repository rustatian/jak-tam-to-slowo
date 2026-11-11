import Foundation
import HTTPTypes
import OpenAPIRuntime

/// Puts the access token on each API request, and signs out when the API answers 401.
struct AuthMiddleware: ClientMiddleware {
    let session: Session

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @concurrent @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var request = request
        request.headerFields[.authorization] = "Bearer " + (try await session.validAccessToken())
        let (response, responseBody) = try await next(request, body, baseURL)
        // The session refreshes the token before it expires, so a 401 means that it was revoked or the user is gone.
        if response.status == .unauthorized {
            await session.signOut()
        }
        return (response, responseBody)
    }
}
