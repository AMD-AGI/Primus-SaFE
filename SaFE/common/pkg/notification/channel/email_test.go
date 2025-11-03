/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

func TestEmailChannel_Send(t *testing.T) {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USERNAME")
	pass := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	useTLSStr := os.Getenv("SMTP_USE_TLS")
	to := os.Getenv("SMTP_TO")

	if host == "" || user == "" || pass == "" || from == "" || to == "" {
		t.Skip("Skipping test: SMTP configuration not provided in environment variables")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port == 0 {
		port = 587
	}
	useTLS := useTLSStr == "true"

	cfg := Config{
		Email: &EmailConfig{
			SMTPHost: host,
			SMTPPort: port,
			Username: user,
			Password: pass,
			From:     from,
			UseTLS:   useTLS,
		},
	}

	email := &EmailChannel{}
	if err := email.Init(cfg); err != nil {
		t.Fatalf("Fail to init EmailChannel: %v", err)
	}

	msg := &model.Message{
		Email: &model.EmailMessage{
			Title:   "EmailChannel Test",
			Content: "This is a test email sent from EmailChannel unit test.\nIf you received this email, the test is successful.",
			To:      []string{to},
		},
	}

	ctx := context.Background()
	if err := email.Send(ctx, msg); err != nil {
		t.Fatalf("Fail to send email: %v", err)
	}

	t.Logf("The email is sent to %s successfully", to)
}
