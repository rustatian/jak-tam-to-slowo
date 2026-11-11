package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/mail"
	"strings"
	"unicode"
)

// Mail is one plain-text email.
type Mail struct {
	To, Subject, Text string
}

// inviteLink is the start of the universal link of an invitation; the token follows it.
const inviteLink = "https://slowo.rustatian.me/invite/"

// validEmail returns the address without the spaces at its ends and in lower case. ok reports that it is one
// bare address of at most 254 ASCII characters with a dot in its domain. Amazon SES takes only 7-bit ASCII in
// the local part of a destination address.
// https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_Destination.html
func validEmail(s string) (address string, ok bool) {
	address = strings.ToLower(strings.TrimSpace(s))
	if len(address) > 254 || strings.IndexFunc(address, func(r rune) bool { return r > unicode.MaxASCII }) >= 0 {
		return "", false
	}
	// ParseAddress also reads the form "Name <address>". Only a bare address comes back as its own text.
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address {
		return "", false
	}
	if _, domain, _ := strings.Cut(address, "@"); !strings.Contains(domain, ".") {
		return "", false
	}
	return address, true
}

// tokenHash returns the form of an invitation token that the database stores: the hex SHA-256 of the token.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// invitationMail writes the invitation email. The two names come from users, so they are only in the plain text.
func invitationMail(to, inviter, group, token string) Mail {
	who := inviter + " zaprasza Cię do grupy"
	if inviter == "" {
		who = "Masz zaproszenie do grupy"
	}
	return Mail{
		To:      to,
		Subject: "Zaproszenie do grupy w aplikacji „Jak tam to słowo?”",
		Text: "Cześć!\n\n" + who + " „" + group + "” w aplikacji „Jak tam to słowo?”.\n\n" +
			"Żeby dołączyć, otwórz ten link na iPhonie:\n" + inviteLink + token + "\n\n" +
			"Link jest ważny przez 7 dni. Jeśli nie spodziewasz się tego zaproszenia, zignoruj tę wiadomość.\n",
	}
}
