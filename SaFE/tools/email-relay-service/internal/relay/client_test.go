package relay

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/config"
	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/history"
	smtpsender "github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/smtp"
)

// startFakeSMTP starts a minimal SMTP server sufficient for net/smtp.SendMail.
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
			go func(c net.Conn) {
				defer c.Close()
				w := bufio.NewWriter(c)
				r := bufio.NewReader(c)
				write := func(s string) { _, _ = w.WriteString(s); _ = w.Flush() }
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
			}(conn)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, func() { _ = ln.Close() }
}

// newClient builds a ClusterClient targeting the given SMTP server and base URL.
func newClient(smtpHost string, smtpPort int, baseURL string) *ClusterClient {
	sender := smtpsender.NewSender(config.SMTPConfig{Host: smtpHost, Port: smtpPort, From: "from@b.com"})
	store := history.NewStore(10)
	cfg := config.ClusterConfig{
		Name:              "c1",
		BaseURL:           baseURL,
		APIPath:           "/api",
		ReconnectInterval: 10 * time.Millisecond,
	}
	return NewClusterClient(cfg, sender, store)
}

func TestNewClusterClientAndStreamURL(t *testing.T) {
	c := newClient("127.0.0.1", 25, "http://host")
	if got := c.streamURL(); got != "http://host/api/stream" {
		t.Errorf("streamURL = %q", got)
	}
}

func TestReadStream(t *testing.T) {
	smtpHost, smtpPort, stopSMTP := startFakeSMTP(t)
	defer stopSMTP()

	// upstream API accepts ack/fail POSTs
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	c := newClient(smtpHost, smtpPort, api.URL)

	body := strings.Join([]string{
		"event: email",
		`data: {"id":1,"recipients":["a@b.com"],"subject":"s","html_content":"<p>x</p>"}`,
		"",
		"event: email",
		`data: {"id":2,"recipients":[],"subject":"s2"}`,
		"",
		"event: email",
		"data: notjson",
		"",
		"event: heartbeat",
		"data: hb",
		"",
		"event: weird",
		"data: w",
		"",
		"data: orphan-without-event",
		"",
	}, "\n")

	err := c.readStream(context.Background(), strings.NewReader(body))
	if err == nil {
		t.Fatal("expected stream-closed error at EOF")
	}

	// the successful email should have produced a 'sent' record
	records := c.store.GetRecords("", 0)
	var sawSent, sawFailed bool
	for _, r := range records {
		if r.Status == "sent" {
			sawSent = true
		}
		if r.Status == "failed" {
			sawFailed = true
		}
	}
	if !sawSent || !sawFailed {
		t.Errorf("expected both sent and failed records, got %+v", records)
	}
}

func TestReadStreamContextCancelled(t *testing.T) {
	c := newClient("127.0.0.1", 25, "http://host")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := "event: email\ndata: {}\n\n"
	if err := c.readStream(ctx, strings.NewReader(body)); err == nil {
		t.Fatal("expected context error")
	}
}

func TestConsume(t *testing.T) {
	// non-200 response yields an error
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("nope"))
	}))
	defer bad.Close()
	c := newClient("127.0.0.1", 25, bad.URL)
	if err := c.consume(context.Background()); err == nil {
		t.Fatal("expected error for non-200 status")
	}

	// 200 response that ends yields a stream-closed error
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: heartbeat\ndata: hb\n\n"))
	}))
	defer ok.Close()
	c2 := newClient("127.0.0.1", 25, ok.URL)
	if err := c2.consume(context.Background()); err == nil {
		t.Fatal("expected stream-closed error")
	}
}

func TestDoPost(t *testing.T) {
	c := newClient("127.0.0.1", 25, "http://host")

	// non-2xx response is handled without panic
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c.doPost(context.Background(), srv.URL, []byte("{}"))

	// invalid URL triggers the request-creation error branch
	c.doPost(context.Background(), "http://%zz", nil)
}

func TestRunCancelled(t *testing.T) {
	c := newClient("127.0.0.1", 25, "http://127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Run(ctx) // should return promptly after the first failed consume
}

func TestRunReconnectThenCancel(t *testing.T) {
	c := newClient("127.0.0.1", 25, "http://127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context timeout")
	}
}
