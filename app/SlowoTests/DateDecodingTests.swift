import Foundation
import HTTPTypes
import OpenAPIRuntime
import Testing

@testable import Slowo

/// Answers every API call with one fixed JSON body.
struct FixedTransport: ClientTransport {
    let body: String

    func send(_ request: HTTPRequest, body: HTTPBody?, baseURL: URL, operationID: String) async throws -> (HTTPResponse, HTTPBody?) {
        (HTTPResponse(status: .ok, headerFields: [.contentType: "application/json"]), HTTPBody(self.body))
    }
}

struct DateDecodingCase: Sendable, CustomTestStringConvertible {
    let name: String
    let aiConsentAt: String?
    let expected: Date?
    let fails: Bool

    var testDescription: String { name }
}

let wholeSecond = try! Date("2026-11-11T10:47:36Z", strategy: .iso8601)

@Test(arguments: [
    DateDecodingCase(name: "microseconds, as Go sends them", aiConsentAt: "2026-11-11T10:47:36.807122Z", expected: wholeSecond.addingTimeInterval(0.807122), fails: false),
    DateDecodingCase(name: "whole seconds, as Go sends them without a fraction", aiConsentAt: "2026-11-11T10:47:36Z", expected: wholeSecond, fails: false),
    DateDecodingCase(name: "no consent", aiConsentAt: nil, expected: nil, fails: false),
    DateDecodingCase(name: "not a date", aiConsentAt: "yesterday", expected: nil, fails: true),
])
func dateDecoding(_ c: DateDecodingCase) async throws {
    let consent = c.aiConsentAt.map { #","aiConsentAt":"\#($0)""# } ?? ""
    let body = #"{"id":"s1","email":"ola@example.com","displayName":"Ola","direction":"ru-pl","reminder":false"# + consent + "}"
    let client = Client(serverURL: URL(string: "https://api.test")!, configuration: .backend, transport: FixedTransport(body: body))

    guard !c.fails else {
        await #expect(throws: (any Error).self) { _ = try await client.getMe().ok.body.json }
        return
    }
    let me = try await client.getMe().ok.body.json
    switch (me.aiConsentAt, c.expected) {
    case (nil, nil):
        break
    case let (got?, want?):
        #expect(abs(got.timeIntervalSince(want)) < 0.000_01)
    default:
        Issue.record("aiConsentAt = \(String(describing: me.aiConsentAt)), want \(String(describing: c.expected))")
    }
}
