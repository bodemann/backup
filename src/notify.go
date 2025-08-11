package main

import (
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
)

var pushoverURL = "https://api.pushover.net/1/messages.json"
var httpClient = http.DefaultClient
var smtpSendMail = smtp.SendMail

func notify(cfg config, subject, message string) {
	if cfg.PushoverToken != "" && cfg.PushoverUser != "" {
		sendPushover(cfg, subject, message)
	}
	if cfg.EmailServer != "" && cfg.EmailUser != "" && cfg.EmailPassword != "" && cfg.EmailFrom != "" && cfg.EmailTo != "" {
		sendEmail(cfg, subject, message)
	}
}

func sendPushover(cfg config, title, message string) {
	data := url.Values{}
	data.Set("token", cfg.PushoverToken)
	data.Set("user", cfg.PushoverUser)
	data.Set("message", message)
	if title != "" {
		data.Set("title", title)
	}
	_, _ = httpClient.PostForm(pushoverURL, data)
}

func sendEmail(cfg config, subject, body string) {
	addr := cfg.EmailServer
	host := strings.Split(addr, ":")[0]
	if !strings.Contains(addr, ":") {
		addr = addr + ":587"
	}
	auth := smtp.PlainAuth("", cfg.EmailUser, cfg.EmailPassword, host)
	msg := fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body)
	_ = smtpSendMail(addr, auth, cfg.EmailFrom, []string{cfg.EmailTo}, []byte(msg))
}
