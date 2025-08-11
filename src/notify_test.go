package main

import (
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"net/url"
	"strings"
	"testing"
)

func TestNotify(t *testing.T) {
	// setup fake Pushover server
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		form = r.Form
	}))
	defer srv.Close()
	oldURL := pushoverURL
	pushoverURL = srv.URL
	defer func() { pushoverURL = oldURL }()

	// stub smtp
	var called bool
	oldSend := smtpSendMail
	smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		called = true
		if addr != "smtp.example:25" {
			t.Fatalf("unexpected addr %s", addr)
		}
		if from != "from" || len(to) != 1 || to[0] != "to" {
			t.Fatalf("unexpected mail args")
		}
		if !strings.Contains(string(msg), "body") {
			t.Fatalf("missing body in message")
		}
		return nil
	}
	defer func() { smtpSendMail = oldSend }()

	cfg := config{
		PushoverToken: "pt",
		PushoverUser:  "pu",
		EmailServer:   "smtp.example:25",
		EmailUser:     "eu",
		EmailPassword: "ep",
		EmailFrom:     "from",
		EmailTo:       "to",
	}

	notify(cfg, "title", "body")

	if form == nil {
		t.Fatalf("pushover not called")
	}
	if form.Get("token") != "pt" || form.Get("user") != "pu" || form.Get("message") != "body" || form.Get("title") != "title" {
		t.Fatalf("unexpected pushover data: %v", form)
	}
	if !called {
		t.Fatalf("smtp not called")
	}
}
