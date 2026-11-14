import Foundation
import Observation
import UserNotifications

/// The part of `UNUserNotificationCenter` that the reminder uses.
protocol Notifications: Sendable {
    func authorizationStatus() async -> UNAuthorizationStatus
    func requestAuthorization(options: UNAuthorizationOptions) async throws -> Bool
    func add(_ request: UNNotificationRequest) async throws
    func removePendingNotificationRequests(withIdentifiers identifiers: [String])
}

extension UNUserNotificationCenter: Notifications {
    func authorizationStatus() async -> UNAuthorizationStatus {
        await notificationSettings().authorizationStatus
    }
}

enum ReminderError: Error, Equatable {
    /// iOS does not allow the notifications of the app.
    case denied
}

/// One local notification at the next due time of the study cards.
@MainActor
@Observable
final class Reminder {
    /// A request with the same identifier replaces the pending one.
    /// https://developer.apple.com/documentation/usernotifications/unnotificationrequest/init(identifier:content:trigger:)#Discussion
    static let id = "reminder"

    /// True only when the last read status of iOS is `.authorized`.
    private(set) var allowed = false

    var isOn: Bool { allowed && profile.me?.reminder == true }

    private let center: any Notifications
    private let profile: Profile
    private let now: () -> Date
    /// The `nextDueAt` of the last queue answer.
    private var dueAt: Date?

    init(center: any Notifications, profile: Profile, now: @escaping () -> Date = Date.init) {
        self.center = center
        self.profile = profile
        self.now = now
    }

    func refresh() async {
        allowed = await center.authorizationStatus() == .authorized
        await apply()
    }

    func update(nextDueAt: Date?) async {
        dueAt = nextDueAt
        await refresh()
    }

    func turnOn() async throws {
        let status = await center.authorizationStatus()
        if status == .notDetermined {
            // https://developer.apple.com/documentation/usernotifications/asking-permission-to-use-notifications#Explicitly-request-authorization-in-context
            allowed = (try? await center.requestAuthorization(options: [.alert, .sound])) ?? false
        } else {
            allowed = status == .authorized
        }
        guard allowed else {
            throw ReminderError.denied
        }
        try await profile.update(.init(reminder: true))
        await apply()
    }

    func turnOff() async throws {
        try await profile.update(.init(reminder: false))
        await apply()
    }

    func clear() {
        dueAt = nil
        center.removePendingNotificationRequests(withIdentifiers: [Reminder.id])
    }

    private func apply() async {
        // The interval must be greater than zero.
        // https://developer.apple.com/documentation/usernotifications/untimeintervalnotificationtrigger/init(timeinterval:repeats:)
        if isOn, let dueAt, dueAt.timeIntervalSince(now()) > 0 {
            let content = UNMutableNotificationContent()
            content.body = "Masz fiszki do powtórki."
            let trigger = UNTimeIntervalNotificationTrigger(timeInterval: dueAt.timeIntervalSince(now()), repeats: false)
            try? await center.add(UNNotificationRequest(identifier: Reminder.id, content: content, trigger: trigger))
        } else {
            center.removePendingNotificationRequests(withIdentifiers: [Reminder.id])
        }
    }
}
