import Foundation
import Testing

@testable import Slowo

struct CognitoCase: Sendable, CustomTestStringConvertible {
    enum Outcome: Equatable, Sendable {
        case tokens(Tokens)
        case done
        case error(CognitoError)
    }

    let name: String
    let call: @Sendable (Cognito) async throws -> Tokens?
    let status: Int
    let answer: String
    let target: String
    let body: String
    let outcome: Outcome

    var testDescription: String { name }
}

let signInBody = #"{"AuthFlow":"USER_PASSWORD_AUTH","ClientId":"client1","AuthParameters":{"USERNAME":"ola@example.com","PASSWORD":"Haslo-1a"}}"#
let signUpBody = #"{"ClientId":"client1","Username":"ola@example.com","Password":"Haslo-1a","ValidationData":[{"Name":"invitation","Value":"T1"}]}"#

let cognitoCases: [CognitoCase] = [
    CognitoCase(
        name: "sign-in sends USER_PASSWORD_AUTH",
        call: { try await $0.signIn(email: " ola@example.com\n", password: "Haslo-1a") },
        status: 200,
        answer: #"{"AuthenticationResult":{"AccessToken":"a1","ExpiresIn":3600,"IdToken":"i1","RefreshToken":"r1","TokenType":"Bearer"},"ChallengeParameters":{}}"#,
        target: "InitiateAuth",
        body: signInBody,
        outcome: .tokens(Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start.addingTimeInterval(3600)))
    ),
    CognitoCase(
        name: "rejected sign-in",
        call: { try await $0.signIn(email: "ola@example.com", password: "Haslo-1a") },
        status: 400,
        answer: #"{"__type":"NotAuthorizedException","message":"Incorrect username or password."}"#,
        target: "InitiateAuth",
        body: signInBody,
        outcome: .error(.service("NotAuthorizedException"))
    ),
    CognitoCase(
        name: "sign-in challenge",
        call: { try await $0.signIn(email: "ola@example.com", password: "Haslo-1a") },
        status: 200,
        answer: #"{"ChallengeName":"NEW_PASSWORD_REQUIRED","Session":"s1","ChallengeParameters":{}}"#,
        target: "InitiateAuth",
        body: signInBody,
        outcome: .error(.unsupportedChallenge("NEW_PASSWORD_REQUIRED"))
    ),
    CognitoCase(
        name: "an answer that does not decode",
        call: { try await $0.signIn(email: "ola@example.com", password: "Haslo-1a") },
        status: 200,
        answer: "<html></html>",
        target: "InitiateAuth",
        body: signInBody,
        outcome: .error(.badResponse)
    ),
    CognitoCase(
        name: "refresh with rotation",
        call: { try await $0.refresh("r1") },
        status: 200,
        answer: #"{"AuthenticationResult":{"AccessToken":"a2","ExpiresIn":3600,"IdToken":"i2","RefreshToken":"r2","TokenType":"Bearer"}}"#,
        target: "GetTokensFromRefreshToken",
        body: #"{"ClientId":"client1","RefreshToken":"r1"}"#,
        outcome: .tokens(Tokens(accessToken: "a2", refreshToken: "r2", expiresAt: start.addingTimeInterval(3600)))
    ),
    CognitoCase(
        name: "refresh without a new refresh token keeps the old one",
        call: { try await $0.refresh("r1") },
        status: 200,
        answer: #"{"AuthenticationResult":{"AccessToken":"a2","ExpiresIn":1800,"IdToken":"i2","TokenType":"Bearer"}}"#,
        target: "GetTokensFromRefreshToken",
        body: #"{"ClientId":"client1","RefreshToken":"r1"}"#,
        outcome: .tokens(Tokens(accessToken: "a2", refreshToken: "r1", expiresAt: start.addingTimeInterval(1800)))
    ),
    CognitoCase(
        name: "forgot password",
        call: {
            try await $0.forgotPassword(email: "Ola@Example.com ")
            return nil
        },
        status: 200,
        answer: #"{"CodeDeliveryDetails":{"AttributeName":"email","DeliveryMedium":"EMAIL","Destination":"O***@E***"}}"#,
        target: "ForgotPassword",
        body: #"{"ClientId":"client1","Username":"Ola@Example.com"}"#,
        outcome: .done
    ),
    CognitoCase(
        name: "confirm forgot password",
        call: {
            try await $0.confirmForgotPassword(email: "ola@example.com", code: "123456", newPassword: "Nowe-Haslo-2b")
            return nil
        },
        status: 200,
        answer: "{}",
        target: "ConfirmForgotPassword",
        body: #"{"ClientId":"client1","Username":"ola@example.com","ConfirmationCode":"123456","Password":"Nowe-Haslo-2b"}"#,
        outcome: .done
    ),
    CognitoCase(
        name: "error type with a namespace and a suffix",
        call: {
            try await $0.confirmForgotPassword(email: "ola@example.com", code: "000000", newPassword: "Nowe-Haslo-2b")
            return nil
        },
        status: 400,
        answer: #"{"__type":"aws.cognito#CodeMismatchException:http://internal","message":"Invalid code provided, please request a code again."}"#,
        target: "ConfirmForgotPassword",
        body: #"{"ClientId":"client1","Username":"ola@example.com","ConfirmationCode":"000000","Password":"Nowe-Haslo-2b"}"#,
        outcome: .error(.service("CodeMismatchException"))
    ),
    CognitoCase(
        name: "revoke the refresh token",
        call: {
            try await $0.revoke("r1")
            return nil
        },
        status: 200,
        answer: "",
        target: "RevokeToken",
        body: #"{"ClientId":"client1","Token":"r1"}"#,
        outcome: .done
    ),
    CognitoCase(
        name: "sign-up sends the invitation as validation data",
        call: {
            try await $0.signUp(email: " ola@example.com\n", password: "Haslo-1a", invitation: "T1")
            return nil
        },
        status: 200,
        answer: #"{"CodeDeliveryDetails":{"AttributeName":"email","DeliveryMedium":"EMAIL","Destination":"o***@e***"},"UserConfirmed":false,"UserSub":"s1"}"#,
        target: "SignUp",
        body: signUpBody,
        outcome: .done
    ),
    CognitoCase(
        name: "a sign-up that the trigger rejects keeps the text",
        call: {
            try await $0.signUp(email: "ola@example.com", password: "Haslo-1a", invitation: "T1")
            return nil
        },
        status: 400,
        answer: #"{"__type":"UserLambdaValidationException","message":"PreSignUp failed with error invitation not found."}"#,
        target: "SignUp",
        body: signUpBody,
        outcome: .error(.trigger("PreSignUp failed with error invitation not found."))
    ),
    CognitoCase(
        name: "confirm sign-up",
        call: {
            try await $0.confirmSignUp(email: " ola@example.com\n", code: "123456")
            return nil
        },
        status: 200,
        answer: "{}",
        target: "ConfirmSignUp",
        body: #"{"ClientId":"client1","Username":"ola@example.com","ConfirmationCode":"123456"}"#,
        outcome: .done
    ),
    CognitoCase(
        name: "send the sign-up code again",
        call: {
            try await $0.resendCode(email: "ola@example.com ")
            return nil
        },
        status: 200,
        answer: #"{"CodeDeliveryDetails":{"AttributeName":"email","DeliveryMedium":"EMAIL","Destination":"o***@e***"}}"#,
        target: "ResendConfirmationCode",
        body: #"{"ClientId":"client1","Username":"ola@example.com"}"#,
        outcome: .done
    ),
]

@Test(arguments: cognitoCases)
func cognito(_ c: CognitoCase) async throws {
    let fake = FakeCognito(status: c.status, answer: c.answer)
    let outcome: CognitoCase.Outcome
    do {
        outcome = try await c.call(fake.cognito(now: { start })).map(CognitoCase.Outcome.tokens) ?? .done
    } catch let error as CognitoError {
        outcome = .error(error)
    }
    #expect(outcome == c.outcome)

    let request = try #require(fake.requests.first)
    #expect(request.httpMethod == "POST")
    #expect(request.url == URL(string: "https://cognito.test/"))
    #expect(request.value(forHTTPHeaderField: "X-Amz-Target") == "AWSCognitoIdentityProviderService." + c.target)
    #expect(request.value(forHTTPHeaderField: "Content-Type") == "application/x-amz-json-1.1")
    let sent = try JSONSerialization.jsonObject(with: try #require(request.httpBody)) as? NSDictionary
    let expected = try JSONSerialization.jsonObject(with: Data(c.body.utf8)) as? NSDictionary
    #expect(sent == expected)
}
