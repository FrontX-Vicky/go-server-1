package communications

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"server_1/internal/core/config"
)

type EmailProvider interface {
	Send(payload providerPayload) (*providerResult, error)
}

type SMTPProvider struct {
	cfg config.EmailConfig
}

func NewSMTPProvider(cfg config.EmailConfig) *SMTPProvider {
	return &SMTPProvider{cfg: cfg}
}

func formatAddress(recipient EmailRecipient) string {
	email := strings.TrimSpace(recipient.Email)
	name := strings.TrimSpace(recipient.Name)
	if name == "" {
		return email
	}
	safeName := strings.ReplaceAll(name, `"`, `'`)
	return fmt.Sprintf(`"%s" <%s>`, safeName, email)
}

func plainRecipientEmails(recipients []EmailRecipient) []string {
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient.Email) != "" {
			out = append(out, strings.TrimSpace(recipient.Email))
		}
	}
	return out
}

func (p *SMTPProvider) Send(payload providerPayload) (*providerResult, error) {
	if strings.TrimSpace(p.cfg.SMTPHost) == "" || strings.TrimSpace(p.cfg.FromEmail) == "" {
		return nil, fmt.Errorf("email provider not configured")
	}
	addr := fmt.Sprintf("%s:%s", p.cfg.SMTPHost, p.cfg.SMTPPort)
	recipients := append([]string{}, plainRecipientEmails(payload.To)...)
	recipients = append(recipients, plainRecipientEmails(payload.CC)...)
	recipients = append(recipients, plainRecipientEmails(payload.BCC)...)
	if len(recipients) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}

	messageID := fmt.Sprintf("<%d.%s@markx>", time.Now().UnixNano(), randomToken(10))
	raw, err := buildMimeMessage(p.cfg, payload, messageID)
	if err != nil {
		return nil, err
	}

	var auth smtp.Auth
	if strings.TrimSpace(p.cfg.SMTPUser) != "" {
		auth = smtp.PlainAuth("", p.cfg.SMTPUser, p.cfg.SMTPPass, p.cfg.SMTPHost)
	}
	if err := smtp.SendMail(addr, auth, p.cfg.FromEmail, recipients, raw); err != nil {
		return nil, err
	}
	return &providerResult{
		MessageID: messageID,
		Response:  "smtp_send_ok",
	}, nil
}

func buildMimeMessage(cfg config.EmailConfig, payload providerPayload, messageID string) ([]byte, error) {
	mixedBoundary := "mixed-" + randomToken(24)
	altBoundary := "alt-" + randomToken(24)
	var buffer bytes.Buffer

	writeHeader(&buffer, "From", formatAddress(EmailRecipient{Name: firstNonEmpty(cfg.FromName, payload.FromName), Email: firstNonEmpty(cfg.FromEmail, payload.FromEmail)}))
	writeHeader(&buffer, "To", strings.Join(formattedRecipients(payload.To), ", "))
	if len(payload.CC) > 0 {
		writeHeader(&buffer, "Cc", strings.Join(formattedRecipients(payload.CC), ", "))
	}
	writeHeader(&buffer, "Subject", payload.Subject)
	writeHeader(&buffer, "MIME-Version", "1.0")
	writeHeader(&buffer, "Message-ID", messageID)
	writeHeader(&buffer, "Content-Type", fmt.Sprintf(`multipart/mixed; boundary="%s"`, mixedBoundary))
	buffer.WriteString("\r\n")

	buffer.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
	buffer.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", altBoundary))

	buffer.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
	buffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buffer.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	buffer.WriteString(payload.BodyText)
	buffer.WriteString("\r\n")

	buffer.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
	buffer.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buffer.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	buffer.WriteString(payload.BodyHTML)
	buffer.WriteString("\r\n")
	buffer.WriteString(fmt.Sprintf("--%s--\r\n", altBoundary))

	for _, attachment := range payload.Attachments {
		buffer.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
		buffer.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", attachment.MimeType, attachment.FileName))
		buffer.WriteString("Content-Transfer-Encoding: base64\r\n")
		buffer.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", attachment.FileName))

		encoded := make([]byte, base64.StdEncoding.EncodedLen(len(attachment.Content)))
		base64.StdEncoding.Encode(encoded, attachment.Content)
		writeBase64Lines(&buffer, encoded)
		buffer.WriteString("\r\n")
	}
	buffer.WriteString(fmt.Sprintf("--%s--\r\n", mixedBoundary))
	return buffer.Bytes(), nil
}

func formattedRecipients(recipients []EmailRecipient) []string {
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient.Email) != "" {
			out = append(out, formatAddress(recipient))
		}
	}
	return out
}

func writeHeader(buffer *bytes.Buffer, key, value string) {
	buffer.WriteString(key)
	buffer.WriteString(": ")
	buffer.WriteString(value)
	buffer.WriteString("\r\n")
}

func writeBase64Lines(buffer *bytes.Buffer, encoded []byte) {
	for len(encoded) > 76 {
		buffer.Write(encoded[:76])
		buffer.WriteString("\r\n")
		encoded = encoded[76:]
	}
	buffer.Write(encoded)
}

func randomToken(n int) string {
	if n <= 0 {
		return "token"
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
