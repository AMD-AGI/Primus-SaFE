package aitopics

import (
	"encoding/json"
	"time"
)

// Request represents the unified request envelope for AI invocation
type Request struct {
	RequestID string          `json:"request_id"`
	Topic     string          `json:"topic"`
	Version   string          `json:"version"`
	Timestamp time.Time       `json:"timestamp"`
	Context   RequestContext  `json:"context"`
	Payload   json.RawMessage `json:"payload"`
}

// RequestContext provides common context information for all topic handlers
type RequestContext struct {
	ClusterID    string `json:"cluster_id"`
	TenantID     string `json:"tenant_id,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	TraceID      string `json:"trace_id,omitempty"`
	ToolEndpoint string `json:"tool_endpoint,omitempty"`
	Locale       string `json:"locale,omitempty"`
}

// Response represents the unified response envelope from AI Agent
type Response struct {
	RequestID string          `json:"request_id"`
	Status    ResponseStatus  `json:"status"`
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// ResponseStatus defines the response status
type ResponseStatus string

const (
	StatusSuccess ResponseStatus = "success"
	StatusError   ResponseStatus = "error"
	StatusPartial ResponseStatus = "partial"
)

// Error codes
const (
	CodeSuccess           = 0
	CodeInvalidRequest    = 1001
	CodeTopicNotSupported = 1002
	CodePayloadInvalid    = 1003
	CodeUnauthorized      = 1004
	CodeInternalError     = 2001
	CodeLLMError          = 2002
	CodeToolCallFailed    = 2003
	CodeTimeout           = 2004
	CodeAgentUnavailable  = 2005
)

// NewRequest creates a new request with the given topic and payload
func NewRequest(topic string, ctx RequestContext, payload interface{}) (*Request, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Request{
		RequestID: generateRequestID(),
		Topic:     topic,
		Version:   CurrentVersion,
		Timestamp: time.Now().UTC(),
		Context:   ctx,
		Payload:   payloadBytes,
	}, nil
}

// NewSuccessResponse creates a successful response
func NewSuccessResponse(requestID string, payload interface{}) (*Response, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Response{
		RequestID: requestID,
		Status:    StatusSuccess,
		Code:      CodeSuccess,
		Message:   "success",
		Timestamp: time.Now().UTC(),
		Payload:   payloadBytes,
	}, nil
}

// NewErrorResponse creates an error response
func NewErrorResponse(requestID string, code int, message string) *Response {
	return &Response{
		RequestID: requestID,
		Status:    StatusError,
		Code:      code,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
}

// UnmarshalPayload unmarshals the request payload into the given type
func (r *Request) UnmarshalPayload(v interface{}) error {
	return json.Unmarshal(r.Payload, v)
}

// UnmarshalPayload unmarshals the response payload into the given type
func (r *Response) UnmarshalPayload(v interface{}) error {
	return json.Unmarshal(r.Payload, v)
}

// IsSuccess checks if the response indicates success
func (r *Response) IsSuccess() bool {
	return r.Status == StatusSuccess && r.Code == CodeSuccess
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	// Use timestamp + random suffix for simplicity
	// In production, use UUID
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// randomString generates a random alphanumeric string
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond) // Ensure different values
	}
	return string(b)
}

