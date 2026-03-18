package email

import (
	"bytes"
	"html/template"
	"strings"
	texttmpl "text/template"
)

type Templates struct {
	baseURL string
}

func NewTemplates(appBaseURL string) *Templates {
	return &Templates{baseURL: strings.TrimRight(strings.TrimSpace(appBaseURL), "/")}
}

type WelcomeData struct {
	Username string
}

type AlertTriggeredData struct {
	Username  string
	Symbol    string
	Currency  string
	Direction string
	Threshold float64
}

type AdminCriticalData struct {
	Title   string
	Message string
}

func (t *Templates) Welcome(d WelcomeData) (subject, textBody, htmlBody string) {
	subject = "Welcome to Blockchain Explorer"
	textT := "Hi {{.Username}},\n\nWelcome to Blockchain Explorer.\n\n- Team\n"
	htmlT := `<p>Hi <strong>{{.Username}}</strong>,</p><p>Welcome to Blockchain Explorer.</p><p>— Team</p>`
	return subject, renderText(textT, d), renderHTML(htmlT, d)
}

func (t *Templates) AlertTriggered(d AlertTriggeredData) (subject, textBody, htmlBody string) {
	subject = "Price alert triggered"
	textT := "Hi {{.Username}},\n\nYour alert for {{.Symbol}} has triggered:\n- Direction: {{.Direction}}\n- Threshold: {{.Threshold}} {{.Currency}}\n\n- Team\n"
	htmlT := `<p>Hi <strong>{{.Username}}</strong>,</p><p>Your alert for <strong>{{.Symbol}}</strong> has triggered:</p><ul><li>Direction: {{.Direction}}</li><li>Threshold: {{.Threshold}} {{.Currency}}</li></ul><p>— Team</p>`
	return subject, renderText(textT, d), renderHTML(htmlT, d)
}

func (t *Templates) AdminCritical(d AdminCriticalData) (subject, textBody, htmlBody string) {
	subject = "CRITICAL: " + d.Title
	textT := "CRITICAL: {{.Title}}\n\n{{.Message}}\n"
	htmlT := `<p><strong>CRITICAL: {{.Title}}</strong></p><pre style="white-space:pre-wrap">{{.Message}}</pre>`
	return subject, renderText(textT, d), renderHTML(htmlT, d)
}

func renderHTML(tpl string, data any) string {
	t, err := template.New("h").Parse(tpl)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	_ = t.Execute(&buf, data)
	return buf.String()
}

func renderText(tpl string, data any) string {
	t, err := texttmpl.New("t").Parse(tpl)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	_ = t.Execute(&buf, data)
	return buf.String()
}

