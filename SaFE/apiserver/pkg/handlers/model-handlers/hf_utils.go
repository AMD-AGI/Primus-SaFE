/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// createHTTPClient creates an HTTP client that skips TLS verification
// This is needed for environments with corporate proxies using self-signed certificates
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

// HFModelMetadata represents the JSON response from Hugging Face API
type HFModelMetadata struct {
	ID       string   `json:"id"`
	Author   string   `json:"author"`
	Tags     []string `json:"tags"`
	Pipeline string   `json:"pipeline_tag"`
}

// HFModelInfo contains the extracted information
type HFModelInfo struct {
	DisplayName string
	Description string
	Icon        string
	Label       string
	Tags        []string
	MaxTokens   int // Maximum context length from config.json (max_position_embeddings)
}

// GetHFModelInfo fetches metadata and readme from Hugging Face to extract model info.
func GetHFModelInfo(urlOrID string) (*HFModelInfo, error) {
	// 1. Parse Repo ID
	repoID := cleanRepoID(urlOrID)
	if repoID == "" {
		return nil, fmt.Errorf("invalid huggingface url or repo id")
	}

	info := &HFModelInfo{}

	// 2. Fetch Metadata from API
	// URL: https://huggingface.co/api/models/{repoID}
	metaURL := fmt.Sprintf("https://huggingface.co/api/models/%s", repoID)
	if err := fetchJSON(metaURL, info); err != nil {
		klog.ErrorS(err, "Failed to fetch HF metadata", "url", metaURL)
		// Don't return error yet, try to get info from README if API fails (though API is primary for Tags/Label)
	}

	// 3. Fetch config.json to get max_position_embeddings (MaxTokens)
	configURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/config.json", repoID)
	if maxTokens, err := fetchMaxTokens(configURL); err == nil {
		info.MaxTokens = maxTokens
		klog.InfoS("Fetched MaxTokens from config.json", "repoID", repoID, "maxTokens", maxTokens)
	} else {
		klog.InfoS("Could not fetch MaxTokens from config.json", "repoID", repoID, "error", err)
	}

	// 4. Fetch README.md content
	// URL: https://huggingface.co/{repoID}/resolve/main/README.md
	readmeURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/README.md", repoID)
	readmeContent, err := fetchText(readmeURL)
	if err != nil {
		klog.ErrorS(err, "Failed to fetch README", "url", readmeURL)
	} else {
		// 5. Extract Description from README
		info.Description = extractDescription(readmeContent)
	}

	// 5. Fetch model page to get author avatar (Icon)
	pageURL := fmt.Sprintf("https://huggingface.co/%s", repoID)
	if pageHTML, err := fetchText(pageURL); err == nil {
		info.Icon = extractIconFromPage(pageHTML)
	} else {
		klog.ErrorS(err, "Failed to fetch model page for icon", "url", pageURL)
	}

	// Fallback/Cleanup
	if info.DisplayName == "" {
		info.DisplayName = repoID
	}
	// If Label (Author) is missing from API, try to extract from RepoID
	if info.Label == "" {
		parts := strings.Split(repoID, "/")
		if len(parts) > 0 {
			info.Label = parts[0]
		}
	}

	return info, nil
}

// cleanRepoID extracts "org/repo" from a full URL or returns the ID as is
func cleanRepoID(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimSuffix(input, "/")

	// Remove protocol and domain
	input = strings.TrimPrefix(input, "https://")
	input = strings.TrimPrefix(input, "http://")
	input = strings.TrimPrefix(input, "huggingface.co/")

	// Handle cases like "huggingface.co/api/models/org/repo" if user copy-pasted api url
	input = strings.TrimPrefix(input, "api/models/")

	return input
}

// fetchJSON fetches and parses JSON into struct
func fetchJSON(url string, info *HFModelInfo) error {
	client := createHTTPClient()
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api returned status: %d", resp.StatusCode)
	}

	var meta HFModelMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return err
	}

	info.DisplayName = meta.ID
	info.Label = meta.Author
	info.Tags = meta.Tags
	return nil
}

// fetchMaxTokens fetches config.json and extracts max_position_embeddings
func fetchMaxTokens(url string) (int, error) {
	client := createHTTPClient()
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("config.json returned status: %d", resp.StatusCode)
	}

	var config map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return 0, err
	}

	// Try different field names that HuggingFace models use for max context length
	fieldNames := []string{
		"max_position_embeddings",
		"n_positions",
		"max_seq_len",
		"max_sequence_length",
		"seq_length",
	}

	for _, field := range fieldNames {
		if val, ok := config[field]; ok {
			switch v := val.(type) {
			case float64:
				return int(v), nil
			case int:
				return v, nil
			}
		}
	}

	return 0, fmt.Errorf("max_position_embeddings not found in config.json")
}

// fetchText fetches raw text content
func fetchText(url string) (string, error) {
	client := createHTTPClient()
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("returned status: %d", resp.StatusCode)
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// extractDescription attempts to find the best introduction text
func extractDescription(readme string) string {
	// Normalize newlines
	readme = strings.ReplaceAll(readme, "\r\n", "\n")

	// 1. Try to find specific headers (case insensitive)
	// We remove "##" prefix from strings to allow dynamic matching of "#" depth and optional numbering
	headers := []string{
		"Model Introduction",
		"Introduction",
		"Model Summary",
		"Overview",
		"Model Card",
	}

	for _, h := range headers {
		// Regex explanation:
		// (?m)        : Multi-line mode (so ^ matches start of line)
		// ^#+         : Line starts with one or more #
		// \s*         : Optional whitespace
		// (\d+\.)?    : Optional numbering (e.g., "1.")
		// \s*         : Optional whitespace
		// %s          : The header text
		// \s*$        : End of the header line (ignoring trailing spaces)
		// \n          : The newline after header
		// ([\s\S]*?)  : Capture content (non-greedy)
		// (\n#|$)     : Stop at next header (newline followed by #) or End of File
		regex := regexp.MustCompile(fmt.Sprintf(`(?im)^#+\s*(\d+\.)?\s*%s\s*$\n([\s\S]*?)(\n#|$)`, regexp.QuoteMeta(h)))
		matches := regex.FindStringSubmatch(readme)
		if len(matches) > 2 {
			// matches[0]: full match
			// matches[1]: numbering group (e.g. "1.")
			// matches[2]: content group
			desc := cleanMarkdown(matches[2])

			// Only take the first paragraph if there are multiple
			if parts := strings.Split(desc, "\n\n"); len(parts) > 0 {
				desc = parts[0]
			}

			if len(desc) > 20 {
				return truncate(desc, 500)
			}
		}
	}

	// 2. Try to find the first substantial HTML <p> paragraph
	// This handles cases where README uses HTML tags for layout
	pRegex := regexp.MustCompile(`(?si)<p>(.*?)</p>`)
	pMatches := pRegex.FindAllStringSubmatch(readme, -1)
	for _, m := range pMatches {
		text := cleanMarkdown(m[1])
		// Only take the first paragraph if there are multiple within <p> (though <p> usually implies one)
		if parts := strings.Split(text, "\n\n"); len(parts) > 0 {
			text = parts[0]
		}
		if len(text) > 50 {
			return truncate(text, 500)
		}
	}

	// 3. Fallback: Take the first substantial paragraph that isn't a badge or title
	lines := strings.Split(readme, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip headers, images, html tags, badges
		if strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "<") ||
			strings.HasPrefix(line, "![") ||
			strings.HasPrefix(line, "[!") ||
			strings.HasPrefix(line, "|") ||
			strings.Contains(line, "shields.io") {
			continue
		}

		// If line looks like normal text and is long enough
		if len(line) > 50 {
			return truncate(cleanMarkdown(line), 500)
		}
	}

	return ""
}

// cleanMarkdown removes basic markdown syntax for cleaner text
func cleanMarkdown(text string) string {
	// Remove links [text](url) -> text
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	text = linkRegex.ReplaceAllString(text, "$1")

	// Remove HTML tags
	htmlRegex := regexp.MustCompile(`<[^>]*>`)
	text = htmlRegex.ReplaceAllString(text, "")

	// Remove formatting chars
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	text = strings.ReplaceAll(text, "`", "")

	return strings.TrimSpace(text)
}

func truncate(text string, length int) string {
	if len(text) <= length {
		return text
	}
	return text[:length] + "..."
}

// extractIconFromPage parses the model page HTML to find the author avatar
func extractIconFromPage(html string) string {
	// Target: <img alt="" class="size-3.5 rounded-sm flex-none select-none" src="...">
	// We look for the specific class combination that HF uses for the small header avatar
	re := regexp.MustCompile(`<img[^>]*class="[^"]*size-3\.5[^"]*rounded-sm[^"]*"[^>]*src="([^"]+)"`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		// Decode HTML entities just in case (though src usually doesn't have them)
		return strings.ReplaceAll(matches[1], "&amp;", "&")
	}
	return ""
}
