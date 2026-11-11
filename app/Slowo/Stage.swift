import Foundation

/// The backend stack that the app talks to: its API and its Cognito app client.
struct Stage: Sendable {
    let apiURL: URL
    let cognitoURL: URL
    let clientID: String

    #if DEBUG
    /// Debug builds use the stack slowo-dev.
    static let current = Stage(
        apiURL: URL(string: "https://api-dev.slowo.rustatian.me")!,
        cognitoURL: URL(string: "https://cognito-idp.eu-central-1.amazonaws.com/")!,
        clientID: "30hp8scub2bp4jcg69lubh5t06"
    )
    #else
    #error("Release builds need the slowo-prod Cognito client, and slowo-prod does not exist yet.")
    #endif
}
