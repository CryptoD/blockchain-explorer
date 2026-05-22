package server

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

var searchPageTmpl = template.Must(template.New("search").Funcs(template.FuncMap{
	"escape": template.HTMLEscapeString,
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} - Blockchain Explorer</title>
<link rel="stylesheet" href="/dist/styles.css">
</head>
<body class="min-h-screen bg-bg text-text">
<header class="w-full bg-bg-secondary shadow">
<div class="max-w-4xl mx-auto px-4 py-3 flex items-center justify-between gap-4">
<a href="/" class="font-semibold text-lg hover:text-primary">Blockchain Explorer</a>
<form action="/bitcoin" method="get" role="search" class="flex flex-1 max-w-lg gap-2">
<label for="q" class="sr-only">Search</label>
<input id="q" name="q" type="search" value="{{.Query}}" class="flex-1 min-w-0 p-2 border border-border rounded bg-bg text-text" placeholder="Block, transaction, or address">
<button type="submit" class="bg-primary text-white px-4 py-2 rounded shrink-0">Search</button>
</form>
</div>
</header>
<main class="max-w-4xl mx-auto px-4 py-8">
{{if .Error}}
<div class="bg-red-50 border border-red-200 rounded-lg p-4 text-red-800" role="alert"><p>{{.Error}}</p></div>
{{else if .ResultType}}
<h1 class="text-2xl font-bold mb-2">{{.Heading}}</h1>
<p class="text-text-secondary text-sm mb-6">Basic result (no JavaScript). <a href="{{.InteractiveURL}}" class="text-primary hover:underline">Open interactive view</a> for charts, export, and watchlist.</p>
<dl class="bg-bg-secondary border border-border rounded-lg p-6 space-y-3">
{{range .Fields}}
<div><dt class="text-sm text-text-secondary">{{.Label}}</dt><dd class="font-mono text-sm break-all mt-1">{{.Value}}</dd></div>
{{end}}
</dl>
{{if .Links}}
<ul class="mt-6 space-y-2">
{{range .Links}}
<li><a href="{{.Href}}" class="text-primary hover:underline">{{.Text}}</a></li>
{{end}}
</ul>
{{end}}
{{else}}
<p class="text-text-secondary">Enter a block height, transaction hash, or address above.</p>
{{end}}
</main>
<noscript><p class="max-w-4xl mx-auto px-4 pb-8 text-sm text-text-secondary">JavaScript is off — search uses this server-rendered page.</p></noscript>
</body>
</html>`))

type searchPageField struct {
	Label string
	Value string
}

type searchPageLink struct {
	Href string
	Text string
}

type searchPageData struct {
	Title           string
	Query           string
	InteractiveURL  string
	Error           string
	ResultType      string
	Heading         string
	Fields          []searchPageField
	Links           []searchPageLink
}

func bitcoinSearchPageHandler(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	htmlRoot := htmlRootForStaticFiles()
	if query == "" {
		c.File(filepath.Join(htmlRoot, "bitcoin.html"))
		return
	}
	if len(query) > 100 {
		renderSearchPage(c, searchPageData{
			Title:  "Search",
			Query:  query,
			Error:  "Query too long.",
		})
		return
	}

	resultType, result, err := explorerSvc.SearchBlockchain(c.Request.Context(), query)
	if err != nil {
		msg := "Search failed. Try again later."
		if err == ErrNotFound {
			msg = "No matching block, transaction, or address was found."
		}
		renderSearchPage(c, searchPageData{
			Title: "Search",
			Query: query,
			Error: msg,
		})
		return
	}

	data := searchPageData{
		Title:          "Search result",
		Query:          query,
		InteractiveURL: "/bitcoin.html?q=" + url.QueryEscape(query),
		ResultType:     resultType,
	}
	switch resultType {
	case "address":
		data.Heading = "Address"
		data.Fields = addressSearchFields(result)
		if addr := mapString(result, "address"); addr != "" {
			data.InteractiveURL = "/bitcoin.html?q=" + url.QueryEscape(addr)
		}
		data.Links = []searchPageLink{{Href: data.InteractiveURL, Text: "Interactive address view"}}
	case "transaction":
		data.Heading = "Transaction"
		data.Fields = transactionSearchFields(result)
		if txid := mapString(result, "txid"); txid != "" {
			data.InteractiveURL = "/bitcoin.html?q=" + url.QueryEscape(txid)
		}
		data.Links = []searchPageLink{{Href: data.InteractiveURL, Text: "Interactive transaction view"}}
	case "block":
		data.Heading = "Block"
		data.Fields = blockSearchFields(result)
		if hash := mapString(result, "hash"); hash != "" {
			data.InteractiveURL = "/bitcoin.html?q=" + url.QueryEscape(hash)
		}
		data.Links = []searchPageLink{{Href: data.InteractiveURL, Text: "Interactive block view"}}
	default:
		data.Error = "Unknown result type."
	}
	renderSearchPage(c, data)
}

func renderSearchPage(c *gin.Context, data searchPageData) {
	c.Header("Cache-Control", "public, max-age=60")
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	_ = searchPageTmpl.Execute(c.Writer, data)
}

func mapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}

func addressSearchFields(m map[string]interface{}) []searchPageField {
	fields := []searchPageField{
		{Label: "Address", Value: mapString(m, "address")},
	}
	if bal := mapString(m, "balance"); bal != "" {
		fields = append(fields, searchPageField{Label: "Balance (BTC)", Value: bal})
	}
	if rcv := mapString(m, "total_received"); rcv != "" {
		fields = append(fields, searchPageField{Label: "Total received", Value: rcv})
	}
	if sent := mapString(m, "total_sent"); sent != "" {
		fields = append(fields, searchPageField{Label: "Total sent", Value: sent})
	}
	if txs, ok := m["transactions"].([]interface{}); ok {
		fields = append(fields, searchPageField{Label: "Transactions", Value: fmt.Sprintf("%d", len(txs))})
	}
	return fields
}

func transactionSearchFields(m map[string]interface{}) []searchPageField {
	return []searchPageField{
		{Label: "Transaction ID", Value: mapString(m, "txid")},
		{Label: "Size (bytes)", Value: mapString(m, "size")},
		{Label: "Confirmations", Value: mapString(m, "confirmations")},
		{Label: "Block hash", Value: mapString(m, "blockhash")},
	}
}

func blockSearchFields(m map[string]interface{}) []searchPageField {
	return []searchPageField{
		{Label: "Height", Value: mapString(m, "height")},
		{Label: "Hash", Value: mapString(m, "hash")},
		{Label: "Time", Value: mapString(m, "time")},
		{Label: "Transactions", Value: mapString(m, "nTx")},
		{Label: "Size (bytes)", Value: mapString(m, "size")},
	}
}
