package llmgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	commoncrypto "github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// Handler manages LLM Gateway API endpoints and the LLM reverse proxy.
type Handler struct {
	accessController *authority.AccessController
	dbClient         dbclient.Interface
	litellmClient    *LiteLLMClient
	crypto           *commoncrypto.Crypto
	proxy            *httputil.ReverseProxy
}

// ── Budget Request/Response types ─────────────────────────────────────────
type SetBudgetRequest struct {
	MaxBudget float64 `json:"max_budget" binding:"required,gt=0"`
}

type BudgetResponse struct {
	UserEmail      string   `json:"user_email"`
	Spend          float64  `json:"spend"`
	MaxBudget      *float64 `json:"max_budget"`
	Remaining      *float64 `json:"remaining"`
	BudgetExceeded bool     `json:"budget_exceeded"`
	UsagePercent   *float64 `json:"usage_percent"`
	Message        string   `json:"message,omitempty"`
}

// ── Tag Usage Response types ──────────────────────────────────────────────
type TagUsageResponse struct {
	UserEmail               string              `json:"user_email"`
	StartDate               string              `json:"start_date"`
	EndDate                 string              `json:"end_date"`
	TotalSpend              float64             `json:"total_spend"`
	TotalRequests           int64               `json:"total_requests"`
	TotalSuccessfulRequests int64               `json:"total_successful_requests"`
	TotalFailedRequests     int64               `json:"total_failed_requests"`
	TotalTokens             int64               `json:"total_tokens"`
	Daily                   []TagUsageDailyEntry `json:"daily"`
	Tags                    []TagUsageItem       `json:"tags"`
	Page                    int                  `json:"page"`
	PageSize                int                  `json:"page_size"`
	Total                   int                  `json:"total"`
	TotalPages              int                  `json:"total_pages"`
}

type TagUsageDailyEntry struct {
	Date  string  `json:"date"`
	Spend float64 `json:"spend"`
}

type TagUsageItem struct {
	TagName          *string `json:"tag_name"`
	Spend            float64 `json:"spend"`
	APIRequests      int64   `json:"api_requests"`
	SuccessfulRequests int64 `json:"successful_requests"`
	FailedRequests   int64   `json:"failed_requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
}

const (
	defaultTagPageSize = 20
	maxTagPageSize     = 100
	maxSpendLogPages   = 50
)

// ── Request/Response types ────────────────────────────────────────────────

type CreateBindingRequest struct {
	ApimKey string `json:"apim_key" binding:"required"`
}

type BindingResponse struct {
	UserEmail   string `json:"user_email"`
	KeyAlias    string `json:"key_alias"`
	HasAPIMKey  bool   `json:"has_apim_key"`
	ApimKeyHint string `json:"apim_key_hint,omitempty"`
	VirtualKey  string `json:"virtual_key,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// ── Summary types ─────────────────────────────────────────────────────────
type SummaryResponse struct {
	UserEmail  string             `json:"user_email"`
	TotalSpend float64            `json:"total_spend"`
	ModelSpend map[string]float64 `json:"model_spend,omitempty"`
}

// ── Usage types ───────────────────────────────────────────────────────────
type UsageResponse struct {
	UserEmail               string            `json:"user_email"`
	TotalSpend              float64           `json:"total_spend"`
	TotalPromptTokens       int64             `json:"total_prompt_tokens"`
	TotalCompletionTokens   int64             `json:"total_completion_tokens"`
	TotalTokens             int64             `json:"total_tokens"`
	TotalAPIRequests        int64             `json:"total_api_requests"`
	TotalSuccessfulRequests int64             `json:"total_successful_requests"`
	TotalFailedRequests     int64             `json:"total_failed_requests"`
	Daily                   []UsageDailyEntry `json:"daily"`
}

type UsageDailyEntry struct {
	Date               string                    `json:"date"`
	Spend              float64                   `json:"spend"`
	PromptTokens       int64                     `json:"prompt_tokens"`
	CompletionTokens   int64                     `json:"completion_tokens"`
	TotalTokens        int64                     `json:"total_tokens"`
	APIRequests        int64                     `json:"api_requests"`
	SuccessfulRequests int64                     `json:"successful_requests"`
	FailedRequests     int64                     `json:"failed_requests"`
	Models             map[string]UsageModelData `json:"models,omitempty"`
}

type UsageModelData struct {
	Spend              float64 `json:"spend"`
	PromptTokens       int64   `json:"prompt_tokens"`
	CompletionTokens   int64   `json:"completion_tokens"`
	APIRequests        int64   `json:"api_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
}

// LiteLLMClient encapsulates LiteLLM management API calls.
type LiteLLMClient struct {
	endpoint   string // e.g. "http://10.32.80.50:4000"
	adminKey   string // LiteLLM Master Key
	teamID     string // Global Team ID
	httpClient *http.Client
}

// ── Request/Response types ────────────────────────────────────────────────
// CreateUserRequest is the request body for POST /user/new
type CreateUserRequest struct {
	UserID        string   `json:"user_id"`
	UserEmail     string   `json:"user_email"`
	Teams         []string `json:"teams,omitempty"`
	AutoCreateKey bool     `json:"auto_create_key"`
}

// CreateKeyRequest is the request body for POST /key/generate
type CreateKeyRequest struct {
	UserID   string            `json:"user_id"`
	TeamID   string            `json:"team_id"`
	Metadata map[string]string `json:"metadata"`
	KeyAlias string            `json:"key_alias"`
}

// CreateKeyResponse is the response from POST /key/generate
type CreateKeyResponse struct {
	Key     string `json:"key"`      // The generated virtual key (sk-xxx)
	KeyName string `json:"key_name"` // Abbreviated display name (sk-...xxxx), for UI display only
	TokenID string `json:"token"`    // Hashed token stored in LiteLLM DB, used as key identifier for update/delete
	Expires string `json:"expires"`  // Expiration time
}

// UpdateKeyRequest is the request body for POST /key/update
type UpdateKeyRequest struct {
	Key      string            `json:"key,omitempty"`
	KeyAlias string            `json:"key_alias,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DeleteKeyRequest is the request body for POST /key/delete
type DeleteKeyRequest struct {
	Keys       []string `json:"keys,omitempty"`        // List of token hashes to delete
	KeyAliases []string `json:"key_aliases,omitempty"` // Alternative: delete by key alias (e.g. user email)
}

// ── Usage types ───────────────────────────────────────────────────────────
// DailyActivityResponse is the response from GET /user/daily/activity
type DailyActivityResponse struct {
	Results  []DailyResult  `json:"results"`
	Metadata ActivityTotals `json:"metadata"`
}

type DailyResult struct {
	Date      string          `json:"date"`
	Metrics   DailyMetrics    `json:"metrics"`
	Breakdown *DailyBreakdown `json:"breakdown,omitempty"`
}

type DailyMetrics struct {
	Spend              float64 `json:"spend"`
	PromptTokens       int64   `json:"prompt_tokens"`
	CompletionTokens   int64   `json:"completion_tokens"`
	TotalTokens        int64   `json:"total_tokens"`
	APIRequests        int64   `json:"api_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
}

type DailyBreakdown struct {
	Models    map[string]MetricWithMetadata `json:"models,omitempty"`
	Providers map[string]MetricWithMetadata `json:"providers,omitempty"`
}

type MetricWithMetadata struct {
	Metrics DailyMetrics `json:"metrics"`
}

// UserInfoResponse is the response from GET /user/info
type UserInfoResponse struct {
	UserID   string            `json:"user_id"`
	UserInfo UserInfoData      `json:"user_info"`
	Keys     []UserInfoKeyData `json:"keys"`
}

type UserInfoData struct {
	Spend      float64            `json:"spend"`
	MaxBudget  *float64           `json:"max_budget"`
	ModelSpend map[string]float64 `json:"model_spend"`
}

type UserInfoKeyData struct {
	Token    string  `json:"token"`
	KeyAlias string  `json:"key_alias"`
	Spend    float64 `json:"spend"`
}

type ActivityTotals struct {
	TotalSpend              float64 `json:"total_spend"`
	TotalPromptTokens       int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens   int64   `json:"total_completion_tokens"`
	TotalAPIRequests        int64   `json:"total_api_requests"`
	TotalSuccessfulRequests int64   `json:"total_successful_requests"`
	TotalFailedRequests     int64   `json:"total_failed_requests"`
}

type litellmError struct {
	StatusCode int
	Body       string
}

// ── Budget & Tag API Types ────────────────────────────────────────────────
// KeyInfoResponse is the relevant subset of GET /key/info response.
type KeyInfoResponse struct {
	Info KeyInfoData `json:"info"`
}

type KeyInfoData struct {
	Spend     float64  `json:"spend"`
	MaxBudget *float64 `json:"max_budget"`
}

// UpdateKeyBudgetRequest is the request body for updating max_budget via POST /key/update.
type UpdateKeyBudgetRequest struct {
	Key       string   `json:"key"`
	MaxBudget *float64 `json:"max_budget"`
}

// SpendLogsResponse is the paginated response from GET /spend/logs/v2.
type SpendLogsResponse struct {
	Data       []SpendLogEntry `json:"data"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// SpendLogEntry represents a single spend log entry.
type SpendLogEntry struct {
	RequestID        string          `json:"request_id"`
	Model            string          `json:"model"`
	Spend            float64         `json:"spend"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	TotalTokens      int64           `json:"total_tokens"`
	RequestTags      json.RawMessage `json:"request_tags"`
	StartTime        string          `json:"startTime"`
	Status           string          `json:"status"`
}
