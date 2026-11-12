import Foundation
import OpenAPIRuntime

/// The password rules of the user pool.
let passwordRules = "Co najmniej 8 znaków, w tym wielka i mała litera, cyfra i znak specjalny."

/// The Polish text for an error of a sign-in, password reset, profile or group call.
func errorMessage(_ error: Error) -> String {
    // The generated API client wraps the transport error.
    let error = (error as? ClientError)?.underlyingError ?? error
    if error is URLError {
        return "Brak połączenia z internetem."
    }
    switch error {
    case ProfileError.rejected, GroupError.badName:
        // Only a name can break a rule that the app does not check first.
        return "Nazwa musi mieć od 1 do 40 znaków."
    case ProfileError.unexpected:
        return "Nie udało się zapisać zmian."
    case GroupError.badEmail:
        return "Nieprawidłowy adres e-mail."
    case GroupError.tooManyInvitations:
        return "Możesz wysłać najwyżej 20 zaproszeń dziennie."
    case GroupError.emailNotSent:
        return "Nie udało się wysłać e-maila. Spróbuj ponownie."
    default:
        break
    }
    guard case CognitoError.service(let type) = error else {
        return "Coś poszło nie tak. Spróbuj ponownie."
    }
    switch type {
    case "NotAuthorizedException":
        return "Nieprawidłowy e-mail lub hasło."
    case "UserNotConfirmedException":
        return "To konto nie jest jeszcze potwierdzone."
    case "PasswordResetRequiredException":
        return "Ustaw nowe hasło: wybierz „Nie pamiętasz hasła?”."
    case "TooManyRequestsException", "LimitExceededException":
        return "Zbyt wiele prób. Spróbuj ponownie później."
    case "CodeMismatchException":
        return "Nieprawidłowy kod."
    case "ExpiredCodeException":
        return "Kod wygasł. Wyślij nowy kod."
    case "InvalidPasswordException":
        return passwordRules
    default:
        return "Coś poszło nie tak. Spróbuj ponownie."
    }
}
