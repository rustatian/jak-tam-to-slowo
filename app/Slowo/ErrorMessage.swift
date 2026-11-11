import Foundation

/// The password rules of the user pool.
let passwordRules = "Co najmniej 8 znaków, w tym wielka i mała litera, cyfra i znak specjalny."

/// The Polish text for an error of a sign-in or password reset call.
func errorMessage(_ error: Error) -> String {
    if error is URLError {
        return "Brak połączenia z internetem."
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
