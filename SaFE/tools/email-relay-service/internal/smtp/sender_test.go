package smtp

import (
	"bufio"
	"net"
	"strings"
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/config"
)

// startFakeSMTP starts a minimal SMTP server sufficient for net/smtp.SendMail.
// It returns the host, port and a stop function.
func startFakeSMTP(t *testing.T) (string, int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSMTP(conn)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, func() { _ = ln.Close() }
}

// serveSMTP handles a single SMTP conversation with canned replies.
func serveSMTP(conn net.Conn) {
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	write := func(s string) {
		_, _ = w.WriteString(s)
		_ = w.Flush()
	}
	write("220 fake ESMTP\r\n")
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if inData {
			if trimmed == "." {
				inData = false
				write("250 OK\r\n")
			}
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "EHLO"), strings.HasPrefix(trimmed, "HELO"):
			write("250 fake\r\n")
		case strings.HasPrefix(trimmed, "DATA"):
			write("354 end with .\r\n")
			inData = true
		case strings.HasPrefix(trimmed, "QUIT"):
			write("221 Bye\r\n")
			return
		default:
			write("250 OK\r\n")
		}
	}
}

// TestNewSender verifies the sender stores its configuration.
func TestNewSender(t *testing.T) {
	s := NewSender(config.SMTPConfig{Host: "h", Port: 25})
	if s.cfg.Host != "h" || s.cfg.Port != 25 {
		t.Errorf("config not stored")
	}
}

// TestSendNoRecipients verifies an error is returned when no recipients are given.
func TestSendNoRecipients(t *testing.T) {
	s := NewSender(config.SMTPConfig{Host: "h", Port: 25, From: "a@b.com"})
	if err := s.Send("c1", nil, "sub", "<p>hi</p>"); err == nil {
		t.Fatal("expected error for no recipients")
	}
}

// TestSendSuccess verifies a full send against the fake SMTP server.
func TestSendSuccess(t *testing.T) {
	host, port, stop := startFakeSMTP(t)
	defer stop()
	s := NewSender(config.SMTPConfig{Host: host, Port: port, From: "from@b.com", FromName: "Relay"})
	if err := s.Send("c1", []string{"to@b.com"}, "Hello", "<p>hi</p>"); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

// TestSendError verifies an error is returned when the SMTP server is unreachable.
func TestSendError(t *testing.T) {
	// port 1 is not connectable; auth path is also exercised via credentials
	s := NewSender(config.SMTPConfig{
		Host:       "127.0.0.1",
		Port:       1,
		From:       "from@b.com",
		User:       "u",
		Credential: "p",
	})
	if err := s.Send("c1", []string{"to@b.com"}, "sub", "<p>hi</p>"); err == nil {
		t.Fatal("expected error for unreachable smtp server")
	}
}
