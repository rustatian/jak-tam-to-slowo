import Foundation
import Observation

enum SessionError: Error {
    case signedOut
}

/// The signed-in state and the tokens of the user.
@MainActor
@Observable
final class Session {
    private(set) var isSignedIn: Bool
    /// The user typed the password in this run of the app; a launch with stored tokens leaves it false.
    private(set) var signedInThisRun = false

    private let cognito: Cognito
    private let store: TokenStore
    private let now: () -> Date
    private var tokens: Tokens?
    private var refreshing: Task<Tokens, Error>?

    /// Cognito answers these when the refresh token is expired, revoked, or rotated out, or the user is deleted.
    private static let signOutErrors: Set = ["NotAuthorizedException", "RefreshTokenReuseException", "UserNotFoundException"]

    init(cognito: Cognito, store: TokenStore, now: @escaping () -> Date = Date.init) {
        self.cognito = cognito
        self.store = store
        self.now = now
        let tokens = try? store.load()
        self.tokens = tokens
        isSignedIn = tokens != nil
    }

    func signIn(email: String, password: String) async throws {
        let tokens = try await cognito.signIn(email: email, password: password)
        try store.save(tokens)
        self.tokens = tokens
        isSignedIn = true
        signedInThisRun = true
    }

    func sendResetCode(email: String) async throws {
        try await cognito.forgotPassword(email: email)
    }

    /// Sets the new password with the code from the email, then signs in with it.
    func resetPassword(email: String, code: String, newPassword: String) async throws {
        try await cognito.confirmForgotPassword(email: email, code: code, newPassword: newPassword)
        try await signIn(email: email, password: newPassword)
    }

    /// Returns an access token with more than 5 minutes left. Callers that come during a refresh wait for it.
    func validAccessToken() async throws -> String {
        guard let tokens else {
            throw SessionError.signedOut
        }
        if tokens.expiresAt.timeIntervalSince(now()) > 5 * 60 {
            return tokens.accessToken
        }
        let refresh = refreshing ?? Task {
            defer { refreshing = nil }
            do {
                let fresh = try await cognito.refresh(tokens.refreshToken)
                guard isSignedIn else {
                    throw SessionError.signedOut
                }
                try store.save(fresh)
                self.tokens = fresh
                return fresh
            } catch CognitoError.service(let type) where Self.signOutErrors.contains(type) {
                signOut()
                throw CognitoError.service(type)
            }
        }
        refreshing = refresh
        return try await refresh.value.accessToken
    }

    /// Revokes the refresh token at Cognito, then signs out. Without network the user is still out.
    func logOut() async {
        if let refreshToken = tokens?.refreshToken {
            try? await cognito.revoke(refreshToken)
        }
        signOut()
    }

    func signOut() {
        try? store.delete()
        tokens = nil
        isSignedIn = false
    }
}
