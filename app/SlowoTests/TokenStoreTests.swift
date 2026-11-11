import Foundation
import Testing

@testable import Slowo

struct TokenStoreCase: Sendable, CustomTestStringConvertible {
    let name: String
    let steps: @Sendable (TokenStore) throws -> Void
    let loaded: Tokens?

    var testDescription: String { name }
}

let first = Tokens(accessToken: "a1", refreshToken: "r1", expiresAt: start)
let second = Tokens(accessToken: "a2", refreshToken: "r2", expiresAt: start.addingTimeInterval(3600))

let tokenStoreCases: [TokenStoreCase] = [
    TokenStoreCase(name: "load from an empty store is nil", steps: { _ in }, loaded: nil),
    TokenStoreCase(name: "save then load", steps: { try $0.save(first) }, loaded: first),
    TokenStoreCase(
        name: "a second save replaces the first",
        steps: {
            try $0.save(first)
            try $0.save(second)
        },
        loaded: second
    ),
    TokenStoreCase(
        name: "delete removes the item",
        steps: {
            try $0.save(first)
            try $0.delete()
        },
        loaded: nil
    ),
]

@Test(arguments: tokenStoreCases)
func tokenStore(_ c: TokenStoreCase) throws {
    let store = TokenStore(service: "test." + UUID().uuidString)
    defer { try? store.delete() }
    try c.steps(store)
    #expect(try store.load() == c.loaded)
}
