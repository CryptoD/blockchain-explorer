package email

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

func buildMIME(from Address, msg Message) (string, error) {
	subject := strings.TrimSpace(msg.Subject)
	if subject == "" {
		subject = "Notification"
	}
	text := strings.TrimSpace(msg.Text)
	html := strings.TrimSpace(msg.HTML)
	if text == "" && html == "" {
		return "", fmt.Errorf("missing email body")
	}

	boundary := randomBoundary()
	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("To: " + msg.To.String() + "\r\n")
	b.WriteString("Subject: " + encodeHeader(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("\r\n")

	if text != "" {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		b.WriteString(text + "\r\n")
	}
	if html != "" {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		b.WriteString(html + "\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return b.String(), nil
}

func randomBoundary() string {
	buf := make([]byte, 18)
	_, _ = rand.Read(buf)
	return "b" + base64.RawURLEncoding.EncodeToString(buf)
}

func encodeHeader(s string) string {
	// Minimal header encoding: return as-is for now. (We avoid pulling in extra deps.)
	// Many SMTP servers/clients handle UTF-8 in Subject well enough.
	return strings.ReplaceAll(s, "\r", "")
}
