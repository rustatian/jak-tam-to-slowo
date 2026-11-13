import Foundation
import OpenAPIRuntime

/// The password rules of the user pool.
let passwordRules = "Co najmniej 8 znaków, w tym wielka i mała litera, cyfra i znak specjalny."

/// The Polish text for an error of a sign-in, sign-up, password reset, profile, group, invitation or card call.
func errorMessage(_ error: Error) -> String {
    // The generated API client wraps the transport error.
    let error = (error as? ClientError)?.underlyingError ?? error
    if error is URLError {
        return "Brak połączenia z internetem."
    }
    switch error {
    case ProfileError.rejected, GroupError.badName, InvitationError.badName:
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
    case InvitationError.notFound:
        return "To zaproszenie jest już nieważne."
    case InvitationError.tooManyTries:
        return "Zbyt wiele prób. Spróbuj ponownie później."
    // The PreSignUp trigger gives "invitation not found" for a dead token.
    case CognitoError.trigger(let text) where text.contains("invitation not found"):
        return "To zaproszenie jest już nieważne."
    case CognitoError.trigger:
        return "Rejestracja nie jest teraz możliwa. Spróbuj ponownie później."
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
    case "UsernameExistsException":
        return "Konto z tym adresem już istnieje. Zaloguj się."
    case "InvalidParameterException":
        return "Nieprawidłowy adres e-mail."
    default:
        return "Coś poszło nie tak. Spróbuj ponownie."
    }
}
