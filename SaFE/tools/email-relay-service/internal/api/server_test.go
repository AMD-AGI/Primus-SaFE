package api

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// newTestServer builds a Server whose sender targets the given SMTP host/port.
func newTestServer(host string, port int) *Server {
	sender := smtpsender.NewSender(config.SMTPConfig{Host: host, Port: port, From: "from@b.com"})
	store := history.NewStore(10)
	store.RegisterCluster("c1", "http://x")
	return NewServer(sender, store)
}

func TestHandleHealth(t *testing.T) {
	srv := newTestServer("127.0.0.1", 25)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestHandleListClusters(t *testing.T) {
	srv := newTestServer("127.0.0.1", 25)
	req := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var clusters []history.ClusterStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &clusters); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(clusters) != 1 {
		t.Errorf("cluster count = %d, want 1", len(clusters))
	}
}

func TestHandleListHistory(t *testing.T) {
	srv := newTestServer("127.0.0.1", 25)

	// empty history returns an empty JSON array, not null
	req := httptest.NewRequest(http.MethodGet, "/api/history?limit=5", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty history body = %q, want []", rec.Body.String())
	}

	// invalid limit falls back to default and still succeeds
	req = httptest.NewRequest(http.MethodGet, "/api/history?cluster=c1&limit=abc", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandleUI(t *testing.T) {
	srv := newTestServer("127.0.0.1", 25)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Email Relay Service") {
		t.Errorf("unexpected UI response: code=%d", rec.Code)
	}

	// unknown path under the catch-all returns 404
	req = httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleTestSendValidation(t *testing.T) {
	srv := newTestServer("127.0.0.1", 25)

	// invalid JSON body
	req := httptest.NewRequest(http.MethodPost, "/api/test-send", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json status = %d, want 400", rec.Code)
	}

	// missing recipients
	req = httptest.NewRequest(http.MethodPost, "/api/test-send", strings.NewReader(`{"to":[]}`))
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing to status = %d, want 400", rec.Code)
	}
}

func TestHandleTestSendFailure(t *testing.T) {
	srv := newTestServer("127.0.0.1", 1) // unreachable SMTP
	req := httptest.NewRequest(http.MethodPost, "/api/test-send", strings.NewReader(`{"to":["x@y.com"]}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("failure status = %d, want 500", rec.Code)
	}
}

func TestHandleTestSendSuccess(t *testing.T) {
	host, port, stop := startFakeSMTP(t)
	defer stop()
	srv := newTestServer(host, port)
	req := httptest.NewRequest(http.MethodPost, "/api/test-send",
		strings.NewReader(`{"cluster":"c1","to":["x@y.com"],"subject":"s","content":"<p>hi</p>"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("success status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"sent"`) {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestBuildTestHTML(t *testing.T) {
	html := buildTestHTML("my-cluster")
	if !strings.Contains(html, "my-cluster") {
		t.Errorf("cluster name not embedded in html")
	}
}
