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
    #expect(!session.signedInThisRun)

    try await session.signIn(email: "ola@example.com", password: "Haslo-1a")

    #expect(session.isSignedIn)
    #expect(session.signedInThisRun)
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

struct LogOutCase: Sendable, CustomTestStringConvertible {
    let name: String
    let fake: FakeCognito

    var testDescription: String { name }
}

@MainActor
@Test(arguments: [
    LogOutCase(name: "log out revokes the refresh token", fake: FakeCognito(answer: "")),
    LogOutCase(name: "log out without network still signs out", fake: FakeCognito(failure: URLError(.notConnectedToInternet))),
])
func logOut(_ c: LogOutCase) async throws {
    let store = TokenStore(service: "test." + UUID().uuidString)
    defer { try? store.delete() }
    try store.save(Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(3600)))
    let session = Session(cognito: c.fake.cognito(now: { start }), store: store, now: { start })

    await session.logOut()

    #expect(c.fake.requests.map { $0.value(forHTTPHeaderField: "X-Amz-Target") } == ["AWSCognitoIdentityProviderService.RevokeToken"])
    #expect(!session.isSignedIn)
    #expect(try store.load() == nil)
}

struct ConfirmSignUpCase: Sendable, CustomTestStringConvertible {
    enum Outcome: Equatable, Sendable {
        case done
        case cognito(CognitoError)
        case offline
    }

    let name: String
    let steps: [FakeCognito.Step]
    let tries: Int
    let targets: [String]
    let outcomes: [Outcome]
    let signedIn: Bool
    let stored: Tokens?

    var testDescription: String { name }
}

extension FakeCognito.Step {
    static let ok = Self()
    static let signIn = Self(answer: #"{"AuthenticationResult":{"AccessToken":"a1","ExpiresIn":3600,"IdToken":"i1","RefreshToken":"r1","TokenType":"Bearer"}}"#)
    static let mismatch = Self(status: 400, answer: #"{"__type":"CodeMismatchException","message":"Invalid verification code provided, please try again."}"#)
    static let expired = Self(status: 400, answer: #"{"__type":"ExpiredCodeException","message":"Invalid code provided, please request a code again."}"#)
    static let unconfirmed = Self(status: 400, answer: #"{"__type":"UserNotConfirmedException","message":"User is not confirmed."}"#)
    static let offline = Self(failure: URLError(.notConnectedToInternet))
}

let signedUp = Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(3600))

let confirmSignUpCases: [ConfirmSignUpCase] = [
    ConfirmSignUpCase(
        name: "confirm, then sign in",
        steps: [.ok, .signIn], tries: 1, targets: ["ConfirmSignUp", "InitiateAuth"],
        outcomes: [.done], signedIn: true, stored: signedUp
    ),
    ConfirmSignUpCase(
        name: "a wrong code stops before the sign-in",
        steps: [.mismatch], tries: 1, targets: ["ConfirmSignUp"],
        outcomes: [.cognito(.service("CodeMismatchException"))], signedIn: false, stored: nil
    ),
    ConfirmSignUpCase(
        name: "a confirmed account signs in after the answer for an old code",
        steps: [.expired, .signIn], tries: 1, targets: ["ConfirmSignUp", "InitiateAuth"],
        outcomes: [.done], signedIn: true, stored: signedUp
    ),
    ConfirmSignUpCase(
        name: "an old code of an account with no confirmation",
        steps: [.expired, .unconfirmed], tries: 1, targets: ["ConfirmSignUp", "InitiateAuth"],
        outcomes: [.cognito(.service("ExpiredCodeException"))], signedIn: false, stored: nil
    ),
    ConfirmSignUpCase(
        name: "no network at the sign-in after the answer for an old code",
        steps: [.expired, .offline], tries: 1, targets: ["ConfirmSignUp", "InitiateAuth"],
        outcomes: [.offline], signedIn: false, stored: nil
    ),
    ConfirmSignUpCase(
        name: "a second try after a failed sign-in",
        steps: [.ok, .offline, .expired, .signIn], tries: 2,
        targets: ["ConfirmSignUp", "InitiateAuth", "ConfirmSignUp", "InitiateAuth"],
        outcomes: [.offline, .done], signedIn: true, stored: signedUp
    ),
]

@MainActor
@Test(arguments: confirmSignUpCases)
func confirmSignUp(_ c: ConfirmSignUpCase) async throws {
    let store = TokenStore(service: "test." + UUID().uuidString)
    defer { try? store.delete() }
    let fake = FakeCognito(steps: c.steps)
    let session = Session(cognito: fake.cognito(now: { start }), store: store, now: { start })

    var outcomes: [ConfirmSignUpCase.Outcome] = []
    for _ in 0..<c.tries {
        do {
            try await session.confirmSignUp(email: "ola@example.com", code: "123456", password: "Haslo-1a")
            outcomes.append(.done)
        } catch let error as CognitoError {
            outcomes.append(.cognito(error))
        } catch is URLError {
            outcomes.append(.offline)
        }
    }

    let targets = fake.requests.compactMap { $0.value(forHTTPHeaderField: "X-Amz-Target")?.split(separator: ".").last.map(String.init) }
    #expect(targets == c.targets)
    #expect(outcomes == c.outcomes)
    #expect(session.isSignedIn == c.signedIn)
    #expect(try store.load() == c.stored)

    for (request, target) in zip(fake.requests, targets) {
        let data = try #require(request.httpBody)
        let body = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        if target == "ConfirmSignUp" {
            #expect(body["ConfirmationCode"] as? String == "123456")
        } else {
            let parameters = body["AuthParameters"] as? [String: String]
            #expect(parameters?["USERNAME"] == "ola@example.com")
            #expect(parameters?["PASSWORD"] == "Haslo-1a")
        }
    }
}
