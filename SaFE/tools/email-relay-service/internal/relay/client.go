package relay

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/config"
	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/history"
	smtpsender "github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/smtp"
)

// EmailEvent mirrors SaFE's EmailOutbox model sent over SSE.
type EmailEvent struct {
	ID          int32    `json:"id"`
	Source      string   `json:"source"`
	Recipients  []string `json:"recipients"`
	Subject     string   `json:"subject"`
	HTMLContent string   `json:"html_content"`
	Status      string   `json:"status"`
	CreatedAt   string   `json:"created_at"`
}

// ClusterClient manages SSE connection to a single cluster.
type ClusterClient struct {
	cfg    config.ClusterConfig
	sender *smtpsender.Sender
	store  *history.Store
	client *http.Client
	logger *slog.Logger
}

func NewClusterClient(cfg config.ClusterConfig, sender *smtpsender.Sender, store *history.Store) *ClusterClient {
	store.RegisterCluster(cfg.Name, cfg.BaseURL)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &ClusterClient{
		cfg:    cfg,
		sender: sender,
		store:  store,
		client: &http.Client{Timeout: 0, Transport: transport},
		logger: slog.With("cluster", cfg.Name),
	}
}

// Run connects to the SSE stream and processes emails. Reconnects on failure.
func (c *ClusterClient) Run(ctx context.Context) {
	for {
		c.logger.Info("Connecting to SSE stream", "url", c.streamURL())
		err := c.consume(ctx)
		if ctx.Err() != nil {
			c.logger.Info("Shutting down")
			return
		}
		c.logger.Error("SSE connection lost, reconnecting",
			"error", err,
			"interval", c.cfg.ReconnectInterval,
		)
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.cfg.ReconnectInterval):
		}
	}
}

func (c *ClusterClient) consume(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.streamURL(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	c.cfg.Auth.ApplyHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Info("SSE stream connected")
	c.store.SetConnected(c.cfg.Name, true)
	err = c.readStream(ctx, resp.Body)
	c.store.SetConnected(c.cfg.Name, false)
	return err
}

func (c *ClusterClient) readStream(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	var eventType string
	var dataBuf bytes.Buffer

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()

		if line == "" {
			// Empty line = end of event
			if eventType != "" && dataBuf.Len() > 0 {
				c.handleEvent(ctx, eventType, dataBuf.String())
			}
			eventType = ""
			dataBuf.Reset()
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(data)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}
	return fmt.Errorf("stream closed by server")
}

func (c *ClusterClient) handleEvent(ctx context.Context, eventType, data string) {
	switch eventType {
	case "email":
		c.handleEmailEvent(ctx, data)
	case "heartbeat":
		c.logger.Debug("Heartbeat received")
	default:
		c.logger.Debug("Unknown event type", "type", eventType)
	}
}

func (c *ClusterClient) handleEmailEvent(ctx context.Context, data string) {
	var event EmailEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		c.logger.Error("Failed to parse email event", "error", err, "data", data[:min(len(data), 200)])
		return
	}

	c.logger.Info("Received email event",
		"id", event.ID,
		"source", event.Source,
		"subject", event.Subject,
		"to", event.Recipients,
	)

	err := c.sender.Send(c.cfg.Name, event.Recipients, event.Subject, event.HTMLContent)
	if err != nil {
		c.logger.Error("Failed to send email, reporting failure",
			"id", event.ID,
			"error", err,
		)
		c.store.AddRecord(c.cfg.Name, event.ID, event.Source, event.Recipients, event.Subject, "failed", err.Error())
		c.reportFail(ctx, event.ID, err.Error())
		return
	}

	c.store.AddRecord(c.cfg.Name, event.ID, event.Source, event.Recipients, event.Subject, "sent", "")
	c.reportAck(ctx, event.ID)
}

func (c *ClusterClient) reportAck(ctx context.Context, id int32) {
	url := fmt.Sprintf("%s%s/%d/ack", c.cfg.BaseURL, c.cfg.APIPath, id)
	c.doPost(ctx, url, nil)
	c.logger.Info("Email acknowledged", "id", id)
}

func (c *ClusterClient) reportFail(ctx context.Context, id int32, errMsg string) {
	url := fmt.Sprintf("%s%s/%d/fail", c.cfg.BaseURL, c.cfg.APIPath, id)
	payload := map[string]string{"error": errMsg}
	body, _ := json.Marshal(payload)
	c.doPost(ctx, url, body)
	c.logger.Warn("Email marked as failed", "id", id, "error", errMsg)
}

func (c *ClusterClient) doPost(ctx context.Context, url string, body []byte) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bodyReader)
	if err != nil {
		c.logger.Error("Failed to create POST request", "url", url, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	c.cfg.Auth.ApplyHeaders(req)

	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		c.logger.Error("POST request failed", "url", url, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		c.logger.Error("POST returned non-2xx",
			"url", url,
			"status", resp.StatusCode,
			"body", string(respBody),
		)
	}
}

func (c *ClusterClient) streamURL() string {
	return fmt.Sprintf("%s%s/stream", c.cfg.BaseURL, c.cfg.APIPath)
}
