package email

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"os"
	"strconv"
)

// Config is read from environment variables.
// SMTP_HOST     — required (e.g. smtp.gmail.com)
// SMTP_PORT     — default 587
// SMTP_USER     — SMTP username / email
// SMTP_PASSWORD — SMTP password / app password
// SMTP_FROM     — sender address (defaults to SMTP_USER)
// APP_URL       — public URL shown in invite links (e.g. https://grpc.mycompany.com)
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	AppURL   string
}

func LoadConfig() *Config {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return nil // email disabled
	}
	port := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	user := os.Getenv("SMTP_USER")
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = user
	}
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "http://localhost:5173"
	}
	return &Config{
		Host:     host,
		Port:     port,
		User:     user,
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     from,
		AppURL:   appURL,
	}
}

func (c *Config) send(to, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)

	msg := buildMIME(c.From, to, subject, htmlBody)

	var auth smtp.Auth
	if c.Password != "" {
		auth = smtp.PlainAuth("", c.User, c.Password, c.Host)
	}

	// Port 465 → implicit TLS, others → STARTTLS
	if c.Port == 465 {
		tlsCfg := &tls.Config{ServerName: c.Host}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		client, err := smtp.NewClient(conn, c.Host)
		if err != nil {
			return err
		}
		defer client.Close()
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
		if err := client.Mail(c.From); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		w.Write([]byte(msg))
		return w.Close()
	}

	// STARTTLS (port 587 / 25)
	return smtp.SendMail(addr, auth, c.From, []string{to}, []byte(msg))
}

func buildMIME(from, to, subject, htmlBody string) string {
	return fmt.Sprintf(
		"From: gRPC Inspector <%s>\r\nTo: %s\r\nSubject: %s\r\n"+
			"MIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, to, subject, htmlBody,
	)
}

// ── Templates ─────────────────────────────────────────────────────────────────

var inviteTmpl = template.Must(template.New("invite").Parse(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0a0b0e;margin:0;padding:40px 0">
  <div style="max-width:520px;margin:0 auto;background:#13141a;border:1px solid #2a2b35;border-radius:12px;overflow:hidden">

    <!-- Header -->
    <div style="background:#0d0e14;padding:28px 36px;border-bottom:1px solid #2a2b35">
      <div style="display:flex;align-items:center;gap:10px">
        <div style="width:32px;height:32px;background:#4ade80;border-radius:8px;display:flex;align-items:center;justify-content:center;font-size:16px">⬡</div>
        <span style="color:#f0f0f0;font-weight:800;font-size:15px">gRPC Inspector</span>
      </div>
    </div>

    <!-- Body -->
    <div style="padding:36px">
      <h2 style="color:#f0f0f0;margin:0 0 10px;font-size:20px;font-weight:800">You've been invited</h2>
      <p style="color:#8b8c9a;margin:0 0 24px;font-size:14px;line-height:1.6">
        <strong style="color:#f0f0f0">{{.InvitedBy}}</strong> has invited you to join
        <strong style="color:#f0f0f0">{{.WorkspaceName}}</strong> on gRPC Inspector
        as an <strong style="color:#4ade80">{{.Role}}</strong>.
      </p>

      <a href="{{.InviteURL}}"
         style="display:inline-block;background:#4ade80;color:#0a0b0e;font-weight:800;font-size:14px;padding:13px 28px;border-radius:8px;text-decoration:none">
        Accept Invite →
      </a>

      <p style="color:#5a5b6a;margin:24px 0 0;font-size:12px;line-height:1.6">
        This invite expires in 7 days. If you don't have an account yet, you'll be asked to create one.<br>
        If you weren't expecting this invite, you can safely ignore this email.
      </p>
    </div>

    <!-- Footer -->
    <div style="padding:20px 36px;border-top:1px solid #2a2b35;background:#0d0e14">
      <p style="color:#5a5b6a;margin:0;font-size:11px">
        gRPC Inspector · <a href="{{.AppURL}}" style="color:#4ade80;text-decoration:none">{{.AppURL}}</a>
      </p>
    </div>
  </div>
</body>
</html>`))

type InviteData struct {
	InvitedBy     string
	WorkspaceName string
	Role          string
	InviteURL     string
	AppURL        string
}

func (c *Config) SendInvite(toEmail string, data InviteData) error {
	var buf bytes.Buffer
	if err := inviteTmpl.Execute(&buf, data); err != nil {
		return err
	}
	subject := fmt.Sprintf("%s invited you to %s on gRPC Inspector", data.InvitedBy, data.WorkspaceName)
	return c.send(toEmail, subject, buf.String())
}

// IsConfigured returns true if SMTP is set up.
func IsConfigured() bool {
	return os.Getenv("SMTP_HOST") != ""
}

// Resolve hostname for display in logs
func Hostname() string {
	h, _ := net.LookupHost(os.Getenv("SMTP_HOST"))
	_ = h
	return os.Getenv("SMTP_HOST")
}
