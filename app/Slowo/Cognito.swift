import Foundation

/// The tokens of the signed-in user. expiresAt is local time, so a wrong device clock does not change it.
struct Tokens: Codable, Equatable, Sendable {
    var accessToken: String
    var refreshToken: String
    var expiresAt: Date
}

enum CognitoError: Error, Equatable {
    /// The error type of a Cognito error answer, for example "NotAuthorizedException".
    case service(String)
    /// Cognito asks for a challenge, for example NEW_PASSWORD_REQUIRED. The app answers none.
    case unsupportedChallenge(String)
    /// A Lambda trigger rejected the request. The text is the message of the answer.
    case trigger(String)
    case badResponse
}

/// The Cognito user pool calls of a public app client: plain JSON over HTTPS, no AWS signature.
/// https://docs.aws.amazon.com/cognito/latest/developerguide/user-pools-API-operations.html
struct Cognito: Sendable {
    typealias Send = @Sendable (URLRequest) async throws -> (Data, URLResponse)

    private let url: URL
    private let clientID: String
    private let send: Send
    private let now: @Sendable () -> Date

    init(
        url: URL,
        clientID: String,
        send: @escaping Send = { try await URLSession.shared.data(for: $0) },
        now: @escaping @Sendable () -> Date = Date.init
    ) {
        self.url = url
        self.clientID = clientID
        self.send = send
        self.now = now
    }

    func signIn(email: String, password: String) async throws -> Tokens {
        let answer: AuthAnswer = try await call("InitiateAuth", [
            "AuthFlow": "USER_PASSWORD_AUTH",
            "ClientId": clientID,
            "AuthParameters": ["USERNAME": trimmed(email), "PASSWORD": password],
        ])
        guard let result = answer.authenticationResult else {
            throw answer.challengeName.map(CognitoError.unsupportedChallenge) ?? CognitoError.badResponse
        }
        guard let refreshToken = result.refreshToken else {
            throw CognitoError.badResponse
        }
        return tokens(result, refreshToken: refreshToken)
    }

    /// Without rotation, the answer has no refresh token, and the old one stays valid.
    func refresh(_ refreshToken: String) async throws -> Tokens {
        let answer: AuthAnswer = try await call("GetTokensFromRefreshToken", [
            "ClientId": clientID,
            "RefreshToken": refreshToken,
        ])
        guard let result = answer.authenticationResult else {
            throw CognitoError.badResponse
        }
        return tokens(result, refreshToken: result.refreshToken ?? refreshToken)
    }

    func forgotPassword(email: String) async throws {
        let _: Empty = try await call("ForgotPassword", [
            "ClientId": clientID,
            "Username": trimmed(email),
        ])
    }

    func confirmForgotPassword(email: String, code: String, newPassword: String) async throws {
        let _: Empty = try await call("ConfirmForgotPassword", [
            "ClientId": clientID,
            "Username": trimmed(email),
            "ConfirmationCode": code,
            "Password": newPassword,
        ])
    }

    /// The PreSignUp trigger reads the invitation token from the validation data.
    func signUp(email: String, password: String, invitation: String) async throws {
        let _: Empty = try await call("SignUp", [
            "ClientId": clientID,
            "Username": trimmed(email),
            "Password": password,
            "ValidationData": [["Name": "invitation", "Value": invitation]],
        ])
    }

    func confirmSignUp(email: String, code: String) async throws {
        let _: Empty = try await call("ConfirmSignUp", [
            "ClientId": clientID,
            "Username": trimmed(email),
            "ConfirmationCode": code,
        ])
    }

    func resendCode(email: String) async throws {
        let _: Empty = try await call("ResendConfirmationCode", [
            "ClientId": clientID,
            "Username": trimmed(email),
        ])
    }

    /// Revokes the refresh token and the access tokens made from it.
    func revoke(_ refreshToken: String) async throws {
        let _: Empty = try await call("RevokeToken", [
            "ClientId": clientID,
            "Token": refreshToken,
        ])
    }

    private func tokens(_ result: AuthAnswer.Result, refreshToken: String) -> Tokens {
        Tokens(
            accessToken: result.accessToken,
            refreshToken: refreshToken,
            expiresAt: now().addingTimeInterval(TimeInterval(result.expiresIn))
        )
    }

    private func trimmed(_ email: String) -> String {
        email.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func call<Answer: Decodable>(_ operation: String, _ body: [String: Any]) async throws -> Answer {
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/x-amz-json-1.1", forHTTPHeaderField: "Content-Type")
        request.setValue("AWSCognitoIdentityProviderService." + operation, forHTTPHeaderField: "X-Amz-Target")
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (data, response) = try await send(request)
        guard let status = (response as? HTTPURLResponse)?.statusCode else {
            throw CognitoError.badResponse
        }
        guard (200..<300).contains(status) else {
            guard let answer = try? JSONDecoder().decode(ErrorAnswer.self, from: data) else {
                throw CognitoError.badResponse
            }
            let name = errorName(answer.type)
            throw name == "UserLambdaValidationException" ? CognitoError.trigger(answer.message ?? "") : CognitoError.service(name)
        }
        do {
            // RevokeToken answers with an empty body.
            return try JSONDecoder().decode(Answer.self, from: data.isEmpty ? Data("{}".utf8) : data)
        } catch {
            throw CognitoError.badResponse
        }
    }

    /// An AWS JSON 1.1 error type can have a namespace before "#" and a suffix after ":".
    /// https://smithy.io/2.0/aws/protocols/aws-json-1_1-protocol.html#operation-error-serialization
    private func errorName(_ type: String) -> String {
        var name = Substring(type)
        if let colon = name.firstIndex(of: ":") {
            name = name[..<colon]
        }
        if let hash = name.firstIndex(of: "#") {
            name = name[name.index(after: hash)...]
        }
        return String(name)
    }
}

private struct AuthAnswer: Decodable {
    struct Result: Decodable {
        let accessToken: String
        let expiresIn: Int
        let refreshToken: String?

        enum CodingKeys: String, CodingKey {
            case accessToken = "AccessToken"
            case expiresIn = "ExpiresIn"
            case refreshToken = "RefreshToken"
        }
    }

    let authenticationResult: Result?
    let challengeName: String?

    enum CodingKeys: String, CodingKey {
        case authenticationResult = "AuthenticationResult"
        case challengeName = "ChallengeName"
    }
}

private struct ErrorAnswer: Decodable {
    let type: String
    let message: String?

    enum CodingKeys: String, CodingKey {
        case type = "__type"
        case message
    }
}

private struct Empty: Decodable {}
