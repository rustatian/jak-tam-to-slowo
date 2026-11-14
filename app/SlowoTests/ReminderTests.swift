import Foundation
import Synchronization
import Testing
import UserNotifications

@testable import Slowo

/// Keeps the calls to the notification center in order.
final class FakeNotifications: Notifications {
    private let state: Mutex<(status: UNAuthorizationStatus, calls: [String])>
    private let grant: Bool

    init(status: UNAuthorizationStatus, grant: Bool = false) {
        state = Mutex((status, []))
        self.grant = grant
    }

    var calls: [String] { state.withLock { $0.calls } }

    func authorizationStatus() async -> UNAuthorizationStatus {
        state.withLock { $0.status }
    }

    func requestAuthorization(options: UNAuthorizationOptions) async throws -> Bool {
        state.withLock {
            $0.calls.append("request")
            $0.status = grant ? .authorized : .denied
        }
        return grant
    }

    func add(_ request: UNNotificationRequest) async throws {
        let seconds = (request.trigger as? UNTimeIntervalNotificationTrigger).map { Int($0.timeInterval) } ?? -1
        state.withLock { $0.calls.append("add \(request.identifier) \(seconds) \(request.content.body)") }
    }

    func removePendingNotificationRequests(withIdentifiers identifiers: [String]) {
        state.withLock { $0.calls.append("remove \(identifiers.joined(separator: ","))") }
    }
}

let reminded = Components.Schemas.Me(id: "s1", email: "ola@example.com", displayName: "", direction: .ruPl, reminder: true, groupId: "g1")
let scheduled = "add reminder 600 Masz fiszki do powtórki."
let removed = "remove reminder"

private let failed = Operations.UpdateMe.Output.internalServerError(.init(body: .json(.init(message: "x"))))

struct ReminderCase: Sendable, CustomTestStringConvertible {
    enum Thrown: Equatable, Sendable {
        case reminder(ReminderError)
        case profile(ProfileError)
    }

    let name: String
    let me: Components.Schemas.Me
    let status: UNAuthorizationStatus
    var grant = false
    var updateMe: [Operations.UpdateMe.Output] = []
    let action: @Sendable @MainActor (Reminder) async throws -> Void
    let error: Thrown?
    let updates: [Components.Schemas.MeUpdate]
    let calls: [String]
    let isOn: Bool

    var testDescription: String { name }
}

let reminderCases: [ReminderCase] = [
    ReminderCase(
        name: "a queue answer with the reminder on schedules the notification at the next due time",
        me: reminded, status: .authorized,
        action: { await $0.update(nextDueAt: later) },
        error: nil, updates: [], calls: [scheduled], isOn: true
    ),
    ReminderCase(
        name: "a queue answer without a next due time removes the notification",
        me: reminded, status: .authorized,
        action: { await $0.update(nextDueAt: nil) },
        error: nil, updates: [], calls: [removed], isOn: true
    ),
    ReminderCase(
        name: "the server value false removes the notification",
        me: grouped, status: .authorized,
        action: { await $0.update(nextDueAt: later) },
        error: nil, updates: [], calls: [removed], isOn: false
    ),
    ReminderCase(
        name: "the server value true without an iOS answer is off and asks nothing",
        me: reminded, status: .notDetermined,
        action: { await $0.update(nextDueAt: later) },
        error: nil, updates: [], calls: [removed], isOn: false
    ),
    ReminderCase(
        name: "the server value true with a refused permission is off",
        me: reminded, status: .denied,
        action: { await $0.update(nextDueAt: later) },
        error: nil, updates: [], calls: [removed], isOn: false
    ),
    ReminderCase(
        name: "a due time that is not later than now removes the notification",
        me: reminded, status: .authorized,
        action: { await $0.update(nextDueAt: start) },
        error: nil, updates: [], calls: [removed], isOn: true
    ),
    ReminderCase(
        name: "turning on asks iOS and saves true with the permission",
        me: grouped, status: .notDetermined, grant: true,
        updateMe: [.ok(.init(body: .json(reminded)))],
        action: {
            await $0.update(nextDueAt: later)
            try await $0.turnOn()
        },
        error: nil, updates: [.init(reminder: true)], calls: [removed, "request", scheduled], isOn: true
    ),
    ReminderCase(
        name: "turning on with the permission saves true without a question",
        me: grouped, status: .authorized,
        updateMe: [.ok(.init(body: .json(reminded)))],
        action: {
            await $0.update(nextDueAt: later)
            try await $0.turnOn()
        },
        error: nil, updates: [.init(reminder: true)], calls: [removed, scheduled], isOn: true
    ),
    ReminderCase(
        name: "a refusal in the iOS question keeps the reminder off and saves nothing",
        me: grouped, status: .notDetermined, grant: false,
        action: {
            await $0.update(nextDueAt: later)
            try await $0.turnOn()
        },
        error: .reminder(.denied), updates: [], calls: [removed, "request"], isOn: false
    ),
    ReminderCase(
        name: "an earlier refusal shows the hint without a question",
        me: grouped, status: .denied,
        action: {
            await $0.update(nextDueAt: later)
            try await $0.turnOn()
        },
        error: .reminder(.denied), updates: [], calls: [removed], isOn: false
    ),
    ReminderCase(
        name: "a failed save on turning on keeps the reminder off",
        me: grouped, status: .authorized,
        updateMe: [failed],
        action: {
            await $0.update(nextDueAt: later)
            try await $0.turnOn()
        },
        error: .profile(.unexpected), updates: [.init(reminder: true)], calls: [removed], isOn: false
    ),
    ReminderCase(
        name: "turning off saves false and removes the notification",
        me: reminded, status: .authorized,
        updateMe: [.ok(.init(body: .json(grouped)))],
        action: {
            await $0.update(nextDueAt: later)
            try await $0.turnOff()
        },
        error: nil, updates: [.init(reminder: false)], calls: [scheduled, removed], isOn: false
    ),
    ReminderCase(
        name: "a failed save on turning off keeps the notification",
        me: reminded, status: .authorized,
        updateMe: [failed],
        action: {
            await $0.update(nextDueAt: later)
            try await $0.turnOff()
        },
        error: .profile(.unexpected), updates: [.init(reminder: false)], calls: [scheduled], isOn: true
    ),
    ReminderCase(
        name: "a removal for no group drops the due time",
        me: reminded, status: .authorized,
        action: {
            await $0.update(nextDueAt: later)
            $0.clear()
            await $0.refresh()
        },
        error: nil, updates: [], calls: [scheduled, removed, removed], isOn: true
    ),
]

@MainActor
@Test(arguments: reminderCases)
func reminder(_ c: ReminderCase) async throws {
    let api = FakeAPI(.init(getMe: [.ok(.init(body: .json(c.me)))], updateMe: c.updateMe))
    let profile = Profile(api: api)
    try await profile.load()
    let center = FakeNotifications(status: c.status, grant: c.grant)
    let reminder = Reminder(center: center, profile: profile, now: { start })

    var thrown: ReminderCase.Thrown?
    do {
        try await c.action(reminder)
    } catch let error as ReminderError {
        thrown = .reminder(error)
    } catch let error as ProfileError {
        thrown = .profile(error)
    }

    #expect(thrown == c.error)
    #expect(api.updates == c.updates)
    #expect(center.calls == c.calls)
    #expect(reminder.isOn == c.isOn)
}
