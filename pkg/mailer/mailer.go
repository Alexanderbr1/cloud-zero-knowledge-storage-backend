// Package mailer sends transactional emails.
//
// Transport selection (checked in order):
//  1. RESEND_API_KEY is set  → Resend HTTP API (no SMTP ports needed)
//  2. SMTP_HOST is set       → SMTP with STARTTLS or SMTPS
//  3. Neither                → no-op, all sends return nil
package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// Config holds connection parameters for both transports.
type Config struct {
	// Resend (preferred — works everywhere, no SMTP ports needed)
	ResendAPIKey string // RESEND_API_KEY

	// SMTP (fallback)
	Host     string // SMTP_HOST
	Port     int    // SMTP_PORT (default 587)
	Username string // SMTP_USERNAME
	Password string // SMTP_PASSWORD
	From     string // SMTP_FROM
	TLS      bool   // SMTP_TLS — true for SMTPS port 465
}

// Mailer sends HTML+plain-text emails and implements the Notifier interfaces
// expected by the auth and sharing usecases.
type Mailer struct {
	cfg        Config
	addr       string
	httpClient *http.Client
}

func New(cfg Config) *Mailer {
	return &Mailer{
		cfg:        cfg,
		addr:       fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ─── Template data ────────────────────────────────────────────────────────────

type loginEmailData struct {
	DeviceName string
	IPAddress  string
	Time       string
}

type shareEmailData struct {
	OwnerEmail string
	FileName   string
}

// ─── Compiled templates ───────────────────────────────────────────────────────

var (
	loginTmpl = template.Must(template.New("login").Parse(loginHTML))
	shareTmpl = template.Must(template.New("share").Parse(shareHTML))
)

// ─── Notification methods ─────────────────────────────────────────────────────

// NotifyNewLogin sends a security alert when a user logs in from a new device.
func (m *Mailer) NotifyNewLogin(ctx context.Context, toEmail, deviceName, ipAddress string) error {
	data := loginEmailData{
		DeviceName: deviceName,
		IPAddress:  ipAddress,
		Time:       time.Now().UTC().Format("02.01.2006 15:04"),
	}
	plain := fmt.Sprintf(
		"Зафиксирован вход в ваш аккаунт.\n\nУстройство: %s\nIP-адрес: %s\nВремя: %s UTC\n\nЕсли это были не вы — немедленно завершите все сессии в разделе «Безопасность».",
		data.DeviceName, data.IPAddress, data.Time,
	)
	var htmlBuf bytes.Buffer
	if err := loginTmpl.Execute(&htmlBuf, data); err != nil {
		return fmt.Errorf("mailer: render login template: %w", err)
	}
	return m.send(ctx, toEmail, "Новый вход в аккаунт", plain, htmlBuf.String())
}

// NotifyNewShare tells a recipient that someone shared a file with them.
func (m *Mailer) NotifyNewShare(ctx context.Context, toEmail, ownerEmail, fileName string) error {
	data := shareEmailData{OwnerEmail: ownerEmail, FileName: fileName}
	plain := fmt.Sprintf(
		"Пользователь %s открыл вам доступ к файлу «%s».\n\nВойдите в приложение и перейдите в раздел «Доступные мне», чтобы скачать файл.",
		data.OwnerEmail, data.FileName,
	)
	var htmlBuf bytes.Buffer
	if err := shareTmpl.Execute(&htmlBuf, data); err != nil {
		return fmt.Errorf("mailer: render share template: %w", err)
	}
	return m.send(ctx, toEmail, "Вам поделились файлом", plain, htmlBuf.String())
}

// ─── Transport selection ──────────────────────────────────────────────────────

func (m *Mailer) send(ctx context.Context, to, subject, plain, html string) error {
	switch {
	case m.cfg.ResendAPIKey != "":
		return m.sendViaResend(ctx, to, subject, plain, html)
	case m.cfg.Host != "":
		return m.sendViaSMTP(to, subject, plain, html)
	default:
		return nil // not configured — silently skip
	}
}

// ─── Resend HTTP API ──────────────────────────────────────────────────────────

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

func (m *Mailer) sendViaResend(ctx context.Context, to, subject, plain, html string) error {
	payload, err := json.Marshal(resendRequest{
		From:    m.cfg.From,
		To:      []string{to},
		Subject: subject,
		Text:    plain,
		HTML:    html,
	})
	if err != nil {
		return fmt.Errorf("mailer: marshal resend request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("mailer: build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mailer: resend request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mailer: resend HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ─── SMTP ─────────────────────────────────────────────────────────────────────

func (m *Mailer) sendViaSMTP(to, subject, plain, html string) error {
	boundary, err := randomBoundary()
	if err != nil {
		return fmt.Errorf("mailer: generate boundary: %w", err)
	}
	msg := buildMultipartMessage(m.cfg.From, to, subject, plain, html, boundary)
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	if m.cfg.TLS {
		return m.sendSMTPS(msg, to, auth)
	}
	return smtp.SendMail(m.addr, auth, m.cfg.From, []string{to}, msg)
}

func (m *Mailer) sendSMTPS(msg []byte, to string, auth smtp.Auth) error {
	conn, err := tls.Dial("tcp", m.addr, &tls.Config{ServerName: m.cfg.Host})
	if err != nil {
		return fmt.Errorf("mailer: tls dial: %w", err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return fmt.Errorf("mailer: smtp client: %w", err)
	}
	defer c.Quit() //nolint:errcheck

	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("mailer: smtp auth: %w", err)
	}
	if err := c.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("mailer: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("mailer: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mailer: write body: %w", err)
	}
	return w.Close()
}

func buildMultipartMessage(from, to, subject, plain, html, boundary string) []byte {
	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString(`Content-Type: multipart/alternative; boundary="` + boundary + `"` + "\r\n")
	buf.WriteString("\r\n")
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(strings.ReplaceAll(plain, "\n", "\r\n"))
	buf.WriteString("\r\n\r\n--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	buf.WriteString(html)
	buf.WriteString("\r\n\r\n--" + boundary + "--\r\n")
	return buf.Bytes()
}

func randomBoundary() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "boundary_" + hex.EncodeToString(b), nil
}

// ─── HTML templates ───────────────────────────────────────────────────────────

const baseHeader = `<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"></head>
<body style="margin:0;padding:0;background-color:#F3F4F6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#F3F4F6;padding:40px 16px;">
  <tr><td align="center">
    <table width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;">
      <tr>
        <td style="background:#1D4ED8;padding:24px 32px;text-align:center;border-radius:12px 12px 0 0;">
          <span style="font-size:26px;">&#9729;&#65039;</span>
          <span style="display:inline-block;vertical-align:middle;color:#ffffff;font-size:18px;font-weight:700;margin-left:10px;letter-spacing:-0.2px;">Cloud Storage</span>
        </td>
      </tr>
      <tr><td style="background:#ffffff;padding:36px 32px 28px;">`

const baseFooter = `</td></tr>
      <tr>
        <td style="background:#ffffff;padding:0 32px 28px;border-radius:0 0 12px 12px;border-top:1px solid #E5E7EB;">
          <p style="margin:20px 0 0;font-size:12px;color:#9CA3AF;text-align:center;line-height:1.6;">Это автоматическое уведомление — пожалуйста, не отвечайте на это письмо.</p>
        </td>
      </tr>
    </table>
  </td></tr>
</table>
</body></html>`

const loginHTML = baseHeader + `
  <table width="100%" cellpadding="0" cellspacing="0" border="0" style="margin-bottom:20px;">
    <tr><td align="center"><div style="display:inline-block;background:#EFF6FF;border-radius:50%;width:60px;height:60px;line-height:60px;text-align:center;font-size:30px;">&#128272;</div></td></tr>
  </table>
  <h1 style="margin:0 0 6px;font-size:22px;font-weight:700;color:#111827;text-align:center;">Новый вход в аккаунт</h1>
  <p style="margin:0 0 28px;font-size:14px;color:#6B7280;text-align:center;">Зафиксирован вход с нового устройства</p>
  <table width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#F9FAFB;border:1px solid #E5E7EB;border-radius:8px;margin-bottom:24px;">
    <tr><td style="padding:14px 18px;border-bottom:1px solid #E5E7EB;">
      <span style="display:block;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.6px;color:#9CA3AF;margin-bottom:4px;">Устройство</span>
      <span style="font-size:15px;font-weight:500;color:#111827;">{{.DeviceName}}</span>
    </td></tr>
    <tr><td style="padding:14px 18px;border-bottom:1px solid #E5E7EB;">
      <span style="display:block;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.6px;color:#9CA3AF;margin-bottom:4px;">IP-адрес</span>
      <span style="font-size:15px;font-weight:500;color:#111827;">{{.IPAddress}}</span>
    </td></tr>
    <tr><td style="padding:14px 18px;">
      <span style="display:block;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.6px;color:#9CA3AF;margin-bottom:4px;">Время</span>
      <span style="font-size:15px;font-weight:500;color:#111827;">{{.Time}} UTC</span>
    </td></tr>
  </table>
  <table width="100%" cellpadding="0" cellspacing="0" border="0">
    <tr><td style="background:#FEF2F2;border:1px solid #FECACA;border-radius:8px;padding:14px 16px;">
      <p style="margin:0;font-size:14px;color:#991B1B;line-height:1.6;">&#9888;&#65039; <strong>Если это были не вы</strong> — немедленно завершите все сессии в разделе «Безопасность».</p>
    </td></tr>
  </table>` + baseFooter

const shareHTML = baseHeader + `
  <table width="100%" cellpadding="0" cellspacing="0" border="0" style="margin-bottom:20px;">
    <tr><td align="center"><div style="display:inline-block;background:#EFF6FF;border-radius:50%;width:60px;height:60px;line-height:60px;text-align:center;font-size:30px;">&#128193;</div></td></tr>
  </table>
  <h1 style="margin:0 0 6px;font-size:22px;font-weight:700;color:#111827;text-align:center;">Вам поделились файлом</h1>
  <p style="margin:0 0 28px;font-size:14px;color:#6B7280;text-align:center;">У вас появился новый файл в разделе «Доступные мне»</p>
  <table width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#F9FAFB;border:1px solid #E5E7EB;border-radius:8px;margin-bottom:24px;">
    <tr><td style="padding:14px 18px;border-bottom:1px solid #E5E7EB;">
      <span style="display:block;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.6px;color:#9CA3AF;margin-bottom:4px;">Файл</span>
      <span style="font-size:15px;font-weight:600;color:#1D4ED8;">{{.FileName}}</span>
    </td></tr>
    <tr><td style="padding:14px 18px;">
      <span style="display:block;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.6px;color:#9CA3AF;margin-bottom:4px;">От кого</span>
      <span style="font-size:15px;font-weight:500;color:#111827;">{{.OwnerEmail}}</span>
    </td></tr>
  </table>
  <table width="100%" cellpadding="0" cellspacing="0" border="0">
    <tr><td style="background:#EFF6FF;border:1px solid #BFDBFE;border-radius:8px;padding:14px 16px;">
      <p style="margin:0;font-size:14px;color:#1E40AF;line-height:1.6;">&#128274; Файл зашифрован и доступен только вам. Для просмотра войдите в приложение.</p>
    </td></tr>
  </table>` + baseFooter
