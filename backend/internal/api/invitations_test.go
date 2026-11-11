package api

import (
	"strings"
	"testing"
)

func TestValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // the address in its stored form; empty when the input is not valid
	}{
		{name: "a plain address passes", input: "ola@example.com", want: "ola@example.com"},
		{name: "spaces and capital letters are removed", input: "  OLA@Example.COM ", want: "ola@example.com"},
		{name: "a plus tag passes", input: "ola+tag@example.com", want: "ola+tag@example.com"},
		{name: "254 characters pass", input: strings.Repeat("a", 242) + "@example.com", want: strings.Repeat("a", 242) + "@example.com"},
		{name: "255 characters are rejected", input: strings.Repeat("a", 243) + "@example.com"},
		{name: "a name with the address is rejected", input: "Ola <ola@example.com>"},
		{name: "angle brackets are rejected", input: "<ola@example.com>"},
		{name: "two addresses are rejected", input: "ola@example.com, adam@example.com"},
		{name: "a domain without a dot is rejected", input: "ola@example"},
		{name: "a missing local part is rejected", input: "@example.com"},
		{name: "letters outside ASCII in the local part are rejected", input: "żółw@example.com"},
		{name: "letters outside ASCII in the domain are rejected", input: "ola@przykład.pl"},
		{name: "an empty text is rejected", input: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := validEmail(tt.input)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("validEmail(%q) = %q, %v; want %q, %v", tt.input, got, ok, tt.want, tt.want != "")
			}
		})
	}
}

func TestInvitationMail(t *testing.T) {
	const rest = "\n\nŻeby dołączyć, otwórz ten link na iPhonie:\nhttps://slowo.rustatian.me/invite/ABCDEFGHIJKLMNOPQRSTUVWXYZ\n\n" +
		"Link jest ważny przez 7 dni. Jeśli nie spodziewasz się tego zaproszenia, zignoruj tę wiadomość.\n"
	tests := []struct {
		name     string
		inviter  string
		wantText string
	}{
		{name: "with the name of the inviter", inviter: "Ola", wantText: "Cześć!\n\nOla zaprasza Cię do grupy „Rodzina” w aplikacji „Jak tam to słowo?”." + rest},
		{name: "without a name", inviter: "", wantText: "Cześć!\n\nMasz zaproszenie do grupy „Rodzina” w aplikacji „Jak tam to słowo?”." + rest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := invitationMail("nowy@example.com", tt.inviter, "Rodzina", "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
			want := Mail{To: "nowy@example.com", Subject: "Zaproszenie do grupy w aplikacji „Jak tam to słowo?”", Text: tt.wantText}
			if got != want {
				t.Errorf("mail = %+v\nwant   %+v", got, want)
			}
		})
	}
}

func TestTokenHash(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		// The digest that `printf abc | shasum -a 256` prints.
		{name: "the SHA-256 digest of abc in lower-case hex", token: "abc", want: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokenHash(tt.token); got != tt.want {
				t.Errorf("tokenHash(%q) = %s, want %s", tt.token, got, tt.want)
			}
		})
	}
}
