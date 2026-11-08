package domain

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	_ "embed"

	htmltemplate "html/template"
	texttemplate "text/template"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

const thousandsGroup = 3

//go:embed templates/letter.html
var htmlSource string

//go:embed templates/letter.txt
var textSource string

//nolint:gochecknoglobals
var (
	htmlLetter = htmltemplate.Must(htmltemplate.New("letter").Parse(htmlSource))
	textLetter = texttemplate.Must(texttemplate.New("letter").Parse(textSource))
)

type Letter struct {
	Subject string
	HTML    string
	Text    string
}

type TradeFacts struct {
	Gave     string
	Received string
	UpdateID string
	BaseURL  string
}

type content struct {
	Subject   string
	Heading   string
	Lines     []string
	Action    string
	ActionURL string
	Footer    string
	ManageURL string
}

func ConfirmationLetter(link string, ttl time.Duration) (Letter, error) {
	return render(content{
		Subject: "Confirm your email for Private OTC Desk",
		Heading: "Confirm your email",
		Lines: []string{
			"Confirm this address to get notifications from Private OTC Desk.",
			"The link works for " + lifetime(ttl) + ". If you did not ask for it, ignore this email.",
		},
		Action:    "Confirm email",
		ActionURL: link,
	})
}

func TradeLetter(facts TradeFacts) (Letter, error) {
	lines := []string{fmt.Sprintf("You gave %s and received %s.", facts.Gave, facts.Received)}

	if facts.UpdateID != "" {
		lines = append(lines, "Canton update id: "+facts.UpdateID)
	}

	return render(content{
		Subject:   "Your trade settled",
		Heading:   "Your trade settled",
		Lines:     lines,
		Action:    "View in Activity",
		ActionURL: facts.BaseURL + "/activity",
		Footer:    "You get this email because trade notifications are on in your profile.",
		ManageURL: facts.BaseURL + "/profile",
	})
}

func FormatAmount(value decimal.Decimal, symbol string) string {
	whole, fraction, _ := strings.Cut(value.String(), ".")

	formatted := groupThousands(whole)
	if fraction != "0" && fraction != "" {
		formatted += "." + fraction
	}

	return formatted + " " + symbol
}

func groupThousands(whole string) string {
	var grouped strings.Builder

	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%thousandsGroup == 0 {
			grouped.WriteByte(',')
		}

		grouped.WriteRune(digit)
	}

	return grouped.String()
}

func lifetime(ttl time.Duration) string {
	if hours := int(ttl.Hours()); hours > 0 {
		if hours == 1 {
			return "1 hour"
		}

		return fmt.Sprintf("%d hours", hours)
	}

	return fmt.Sprintf("%d minutes", int(ttl.Minutes()))
}

func render(c content) (Letter, error) {
	var html, text bytes.Buffer

	err := htmlLetter.Execute(&html, c)
	if err != nil {
		return Letter{}, fmt.Errorf("htmlLetter.Execute: %w", err)
	}

	err = textLetter.Execute(&text, c)
	if err != nil {
		return Letter{}, fmt.Errorf("textLetter.Execute: %w", err)
	}

	return Letter{Subject: c.Subject, HTML: html.String(), Text: text.String()}, nil
}
