package channel

import (
	"context"
	"fmt"

	"gopkg.in/gomail.v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

type EmailChannel struct {
	cfg *EmailConfig
}

// Name returns the name of the client factory.
func (e *EmailChannel) Name() string {
	return model.ChannelEmail
}

// Init initializes the notification channel with the provided configuration.
func (e *EmailChannel) Init(cfg Config) error {
	if cfg.Email == nil {
		return fmt.Errorf("email config not provided")
	}
	e.cfg = cfg.Email
	return nil
}

// Send sends a message through the notification channel.
func (e *EmailChannel) Send(ctx context.Context, message *model.Message) error {
	if e.cfg == nil {
		return fmt.Errorf("email channel not initialized")
	}
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	msg := message.Email
	if len(msg.To) == 0 {
		return fmt.Errorf("no recipients provided for email")
	}

	// 创建 gomail 消息
	m := gomail.NewMessage()
	m.SetHeader("From", e.cfg.From)
	m.SetHeader("To", msg.To...)
	m.SetHeader("Subject", msg.Title)
	m.SetBody("text/html", msg.Content)

	// 创建 gomail Dialer
	d := gomail.NewDialer(e.cfg.SMTPHost, e.cfg.SMTPPort, e.cfg.Username, e.cfg.Password)
	d.SSL = e.cfg.UseTLS // true = 465 直连 SSL, false = 587 STARTTLS

	// 发送邮件
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
