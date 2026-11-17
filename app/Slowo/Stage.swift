import Foundation

/// The backend stack that the app talks to: its API and its Cognito app client.
struct Stage: Sendable {
    let apiURL: URL
    let cognitoURL: URL
    let clientID: String

    /// The production stack, the only stack.
    static let current = Stage(
        apiURL: URL(string: "https://api.slowo.rustatian.me")!,
        cognitoURL: URL(string: "https://cognito-idp.eu-central-1.amazonaws.com/")!,
        clientID: "30hp8scub2bp4jcg69lubh5t06"
    )
}
