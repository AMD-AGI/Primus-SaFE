package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/history"
	smtpsender "github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/smtp"
)

type Server struct {
	sender *smtpsender.Sender
	store  *history.Store
	mux    *http.ServeMux
}

func NewServer(sender *smtpsender.Sender, store *history.Store) *Server {
	s := &Server{sender: sender, store: store, mux: http.NewServeMux()}

	s.mux.HandleFunc("GET /", s.handleUI)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /api/test-send", s.handleTestSend)
	s.mux.HandleFunc("GET /api/clusters", s.handleListClusters)
	s.mux.HandleFunc("GET /api/history", s.handleListHistory)

	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.store.GetClusterStatuses())
}

func (s *Server) handleListHistory(w http.ResponseWriter, r *http.Request) {
	cluster := r.URL.Query().Get("cluster")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	w.Header().Set("Content-Type", "application/json")
	records := s.store.GetRecords(cluster, limit)
	if records == nil {
		records = []history.Record{}
	}
	json.NewEncoder(w).Encode(records)
}

type TestSendRequest struct {
	Cluster string   `json:"cluster"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Content string   `json:"content"`
}

func (s *Server) handleTestSend(w http.ResponseWriter, r *http.Request) {
	var req TestSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	if len(req.To) == 0 {
		http.Error(w, `{"error":"to is required"}`, http.StatusBadRequest)
		return
	}
	if req.Subject == "" {
		req.Subject = "Email Relay Test"
	}
	if req.Cluster == "" {
		req.Cluster = "test"
	}
	if req.Content == "" {
		req.Content = buildTestHTML(req.Cluster)
	}

	slog.Info("Test send requested", "cluster", req.Cluster, "to", req.To, "subject", req.Subject)

	err := s.sender.Send(req.Cluster, req.To, req.Subject, req.Content)
	if err != nil {
		slog.Error("Test send failed", "error", err)
		s.store.AddRecord(req.Cluster, 0, "test", req.To, req.Subject, "failed", err.Error())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	s.store.AddRecord(req.Cluster, 0, "test", req.To, req.Subject, "sent", "")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

func buildTestHTML(cluster string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#f4f4f4;font-family:Arial,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="padding:20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.1);">
<tr><td style="background:linear-gradient(135deg,#e67e22,#d35400);padding:24px 30px;">
<h1 style="margin:0;color:#fff;font-size:20px;">Primus-SaFE Email Relay</h1>
<p style="margin:8px 0 0;color:rgba(255,255,255,0.85);font-size:13px;">Test notification from cluster: <strong>%s</strong></p>
</td></tr>
<tr><td style="padding:24px 30px;">
<p style="margin:0 0 12px;font-size:15px;color:#333;">This is a <strong>test email</strong> sent via the Email Relay Service.</p>
<table width="100%%" style="background:#f8f9fa;border-radius:6px;margin:16px 0;">
<tr><td style="padding:14px 18px;">
<p style="margin:0 0 6px;font-size:12px;color:#6c757d;text-transform:uppercase;">Relay Details</p>
<p style="margin:0;font-size:14px;color:#333;"><strong>SMTP:</strong> atlsmtp10.amd.com:25</p>
<p style="margin:4px 0 0;font-size:14px;color:#333;"><strong>From:</strong> primus-safe@amd.com</p>
<p style="margin:4px 0 0;font-size:14px;color:#333;"><strong>Cluster:</strong> %s</p>
</td></tr></table>
<p style="margin:16px 0 0;font-size:13px;color:#6c757d;">If you received this, the email relay is working correctly.</p>
</td></tr>
<tr><td style="background:#f8f9fa;padding:16px 30px;border-top:1px solid #e9ecef;">
<p style="margin:0;font-size:11px;color:#adb5bd;">Generated by Primus-SaFE Email Relay Service</p>
</td></tr>
</table>
</td></tr></table>
</body></html>`, cluster, cluster)
}
