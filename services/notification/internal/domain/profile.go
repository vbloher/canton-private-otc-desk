package domain

import (
	"net/mail"
	"strings"
	"time"
)

const (
	maxEmailLength   = 254
	gmailHost        = "gmail.com"
	googlemailHost   = "googlemail.com"
	forbiddenInEmail = " \t\r\n\"<>,/?#%\\"
)

type EmailStatus string

const (
	EmailNone        EmailStatus = "none"
	EmailUnconfirmed EmailStatus = "unconfirmed"
	EmailConfirmed   EmailStatus = "confirmed"
)

type Profile struct {
	PartyID     string
	Email       string
	ConfirmedAt *time.Time
	News        bool
	Trades      bool
}

func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))

	if email == "" || len(email) > maxEmailLength || strings.ContainsAny(email, forbiddenInEmail) {
		return "", Invalidf("enter a valid email address")
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", Invalidf("enter a valid email address")
	}

	_, host, found := strings.Cut(email, "@")
	if !found || !strings.Contains(host, ".") {
		return "", Invalidf("enter a valid email address")
	}

	return email, nil
}

func CanonicalEmail(email string) string {
	local, host, found := strings.Cut(email, "@")
	if !found {
		return email
	}

	local, _, _ = strings.Cut(local, "+")

	if host == gmailHost || host == googlemailHost {
		return strings.ReplaceAll(local, ".", "") + "@" + gmailHost
	}

	return local + "@" + host
}

func (p Profile) Status() EmailStatus {
	switch {
	case p.Email == "":
		return EmailNone
	case p.ConfirmedAt == nil:
		return EmailUnconfirmed
	default:
		return EmailConfirmed
	}
}

func (p Profile) Confirmed() bool {
	return p.Status() == EmailConfirmed
}

func (p Profile) WantsTrades() bool {
	return p.Confirmed() && p.Trades
}

func (p Profile) WithEmail(email string) Profile {
	p.Email = email
	p.ConfirmedAt = nil

	return p
}
