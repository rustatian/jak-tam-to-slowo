import Foundation
import HTTPTypes
import OpenAPIRuntime

/// Puts the access token on each API request, and signs out when the API answers 401.
/// The invitation preview goes out with no access token: the token of the link is the proof, and a signed-out person calls it too.
struct AuthMiddleware: ClientMiddleware {
    let session: Session

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @concurrent @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        if operationID == Operations.PreviewInvitation.id {
            return try await next(request, body, baseURL)
        }
        var request = request
        request.headerFields[.authorization] = "Bearer " + (try await session.validAccessToken())
        let (response, responseBody) = try await next(request, body, baseURL)
        // The session refreshes the token before it expires, so a 401 means that API Gateway no longer accepts this token.
        if response.status == .unauthorized {
            await session.signOut()
        }
        return (response, responseBody)
    }
}
