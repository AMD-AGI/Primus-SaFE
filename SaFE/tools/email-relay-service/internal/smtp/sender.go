package smtp

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/config"
)

type Sender struct {
	cfg config.SMTPConfig
}

func NewSender(cfg config.SMTPConfig) *Sender {
	return &Sender{cfg: cfg}
}

// Send sends an HTML email. clusterName is prepended to the subject as a tag.
func (s *Sender) Send(clusterName string, recipients []string, subject, htmlContent string) error {
	if len(recipients) == 0 {
		return fmt.Errorf("no recipients")
	}

	taggedSubject := fmt.Sprintf("[%s] %s", clusterName, subject)

	fromHeader := s.cfg.From
	if s.cfg.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", s.cfg.FromName, s.cfg.From)
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(recipients, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", taggedSubject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlContent)

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	var auth smtp.Auth
	if s.cfg.User != "" && s.cfg.Credential != "" {
		auth = smtp.PlainAuth("", s.cfg.User, s.cfg.Credential, s.cfg.Host)
	}

	err := smtp.SendMail(addr, auth, s.cfg.From, recipients, []byte(msg.String()))
	if err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}

	slog.Info("Email sent",
		"cluster", clusterName,
		"to", recipients,
		"subject", taggedSubject,
	)
	return nil
}
