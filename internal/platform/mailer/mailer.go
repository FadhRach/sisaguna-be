// Package mailer mengirim email transaksional (kode lupa password). Tiga mode lewat EMAIL_MODE:
// smtp (kirim sungguhan), log (cetak isi email ke log server, hanya untuk uji), off (tidak mengirim apa pun).
package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Config struct {
	Mode     string // smtp | log | off
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

func New(c Config) Sender {
	switch c.Mode {
	case "smtp":
		return &SMTP{c: c}
	case "log":
		return Log{}
	}
	return Off{}
}

// Off membuang email. Pemanggil tidak tahu bedanya, jadi endpoint lupa password tetap menjawab sama.
type Off struct{}

func (Off) Send(context.Context, string, string, string) error {
	slog.Warn("email tidak dikirim: EMAIL_MODE=off (atur EMAIL_MODE=smtp + SMTP_*, atau log untuk uji)")
	return nil
}

// Log mencetak email ke log. Hanya untuk development / VPS uji: isi email (kode reset) terlihat di log.
type Log struct{}

func (Log) Send(_ context.Context, to, subject, body string) error {
	slog.Info("email (mode log)", "to", to, "subject", subject, "body", body)
	return nil
}

// SMTP memakai net/smtp: STARTTLS otomatis kalau server menawarkannya (port 587). shortcut: tidak mendukung TLS
// implisit (port 465) dan tanpa antrean/retry; ganti ke layanan email transaksional kalau volume atau keandalan perlu naik.
type SMTP struct{ c Config }

func (s *SMTP) Send(ctx context.Context, to, subject, body string) error {
	if strings.ContainsAny(to+subject+s.c.From, "\r\n") {
		return fmt.Errorf("mailer: header tidak valid")
	}
	msg := "From: " + s.c.From + "\r\nTo: " + to + "\r\nSubject: " + subject +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	var auth smtp.Auth
	if s.c.User != "" {
		auth = smtp.PlainAuth("", s.c.User, s.c.Password, s.c.Host)
	}
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(s.c.Host+":"+s.c.Port, auth, s.c.From, []string{to}, []byte(msg)) }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("mailer smtp: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("mailer smtp: %w", ctx.Err())
	}
}
