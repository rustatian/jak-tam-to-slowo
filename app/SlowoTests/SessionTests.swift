import Foundation
import Testing

@testable import Slowo

struct SessionCase: Sendable, CustomTestStringConvertible {
    enum Outcome: Equatable, Sendable {
        case token(String)
        case cognito(CognitoError)
        case offline
    }

    let name: String
    let expiresIn: TimeInterval
    let fake: FakeCognito
    let callers: Int
    let outcome: Outcome
    let refreshCalls: Int
    let signedIn: Bool
    let stored: Tokens?

    var testDescription: String { name }
}

let rotated = #"{"AuthenticationResult":{"AccessToken":"a2","ExpiresIn":3600,"IdToken":"i2","RefreshToken":"r2","TokenType":"Bearer"}}"#
let refreshed = Tokens(accessToken: "a2", refreshToken: "r2", expiresAt: start.addingTimeInterval(3600))

let sessionCases: [SessionCase] = [
    SessionCase(
        name: "fresh token is used without a refresh",
        expiresIn: 30 * 60, fake: FakeCognito(answer: rotated), callers: 1,
        outcome: .token("a1"), refreshCalls: 0, signedIn: true,
        stored: Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(30 * 60))
    ),
    SessionCase(
        name: "expiring token is refreshed and saved",
        expiresIn: 4 * 60, fake: FakeCognito(answer: rotated), callers: 1,
        outcome: .token("a2"), refreshCalls: 1, signedIn: true, stored: refreshed
    ),
    SessionCase(
        name: "two callers share one refresh",
        expiresIn: 4 * 60, fake: FakeCognito(answer: rotated, delay: .milliseconds(50)), callers: 2,
        outcome: .token("a2"), refreshCalls: 1, signedIn: true, stored: refreshed
    ),
    SessionCase(
        name: "rejected refresh signs out",
        expiresIn: 4 * 60, fake: FakeCognito(status: 400, answer: #"{"__type":"NotAuthorizedException"}"#), callers: 1,
        outcome: .cognito(.service("NotAuthorizedException")), refreshCalls: 1, signedIn: false, stored: nil
    ),
    SessionCase(
        name: "reused refresh token signs out",
        expiresIn: 4 * 60, fake: FakeCognito(status: 400, answer: #"{"__type":"RefreshTokenReuseException"}"#), callers: 1,
        outcome: .cognito(.service("RefreshTokenReuseException")), refreshCalls: 1, signedIn: false, stored: nil
    ),
    SessionCase(
        name: "deleted user signs out",
        expiresIn: 4 * 60, fake: FakeCognito(status: 400, answer: #"{"__type":"UserNotFoundException"}"#), callers: 1,
        outcome: .cognito(.service("UserNotFoundException")), refreshCalls: 1, signedIn: false, stored: nil
    ),
    SessionCase(
        name: "network failure keeps the session",
        expiresIn: 4 * 60, fake: FakeCognito(failure: URLError(.notConnectedToInternet)), callers: 1,
        outcome: .offline, refreshCalls: 1, signedIn: true,
        stored: Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(4 * 60))
    ),
]

private func outcome(_ call: () async throws -> String) async -> SessionCase.Outcome {
    do {
        return .token(try await call())
    } catch let error as CognitoError {
        return .cognito(error)
    } catch {
        return .offline
    }
}

@MainActor
@Test(arguments: sessionCases)
func session(_ c: SessionCase) async throws {
    let store = TokenStore(service: "test." + UUID().uuidString)
    defer { try? store.delete() }
    try store.save(Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(c.expiresIn)))
    let session = Session(cognito: c.fake.cognito(now: { start }), store: store, now: { start })

    let outcomes = await withTaskGroup(of: SessionCase.Outcome.self) { group in
        for _ in 0..<c.callers {
            group.addTask { await outcome { try await session.validAccessToken() } }
        }
        var outcomes: [SessionCase.Outcome] = []
        for await outcome in group {
            outcomes.append(outcome)
        }
        return outcomes
    }

    #expect(outcomes == Array(repeating: c.outcome, count: c.callers))
    #expect(c.fake.requests.count == c.refreshCalls)
    #expect(session.isSignedIn == c.signedIn)
    #expect(try store.load() == c.stored)
}

@MainActor
@Test func signInSavesTheTokens() async throws {
    let store = TokenStore(service: "test." + UUID().uuidString)
    defer { try? store.delete() }
    let fake = FakeCognito(answer: #"{"AuthenticationResult":{"AccessToken":"a1","ExpiresIn":3600,"IdToken":"i1","RefreshToken":"r1","TokenType":"Bearer"}}"#)
    let session = Session(cognito: fake.cognito(now: { start }), store: store, now: { start })
    #expect(!session.isSignedIn)

    try await session.signIn(email: "ola@example.com", password: "Haslo-1a")

    #expect(session.isSignedIn)
    #expect(try store.load() == Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(3600)))
}

@MainActor
@Test func signedOutHasNoToken() async throws {
    let store = TokenStore(service: "test." + UUID().uuidString)
    let session = Session(cognito: FakeCognito().cognito(now: { start }), store: store, now: { start })

    await #expect(throws: SessionError.signedOut) {
        try await session.validAccessToken()
    }
}
