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

// ============================================================================
// Model Tag Classification System
// Categories: Provider, Task/Modality, Model Size, Precision/Quantization, Architecture, Training Method
// ============================================================================

// TagCategory represents the category type for model tags
type TagCategory string

const (
	TagCategoryProvider     TagCategory = "provider"     // Blue - Brand, trust, professional
	TagCategoryTask         TagCategory = "task"         // Green - Function, capability, action
	TagCategorySize         TagCategory = "size"         // Purple - Scale, magnitude
	TagCategoryPrecision    TagCategory = "precision"    // Orange - Technical optimization, performance
	TagCategoryArchitecture TagCategory = "architecture" // Red - Core, structural importance
	TagCategoryTraining     TagCategory = "training"     // Yellow - Method, process
	TagCategoryOther        TagCategory = "other"        // Gray - Unclassified, secondary
)

// TagColor represents the color enum for frontend display
type TagColor string

const (
	TagColorBlue   TagColor = "blue"   // Provider
	TagColorGreen  TagColor = "green"  // Task
	TagColorPurple TagColor = "purple" // Size
	TagColorOrange TagColor = "orange" // Precision
	TagColorRed    TagColor = "red"    // Architecture
	TagColorYellow TagColor = "yellow" // Training
	TagColorGray   TagColor = "gray"   // Other
)

// TagWithCategory represents a tag with its color information for frontend display
type TagWithCategory struct {
	Value string   `json:"value"` // Original tag value
	Color TagColor `json:"color"` // Color for frontend display
}

// categoryColorMap maps category to color
var categoryColorMap = map[TagCategory]TagColor{
	TagCategoryProvider:     TagColorBlue,
	TagCategoryTask:         TagColorGreen,
	TagCategorySize:         TagColorPurple,
	TagCategoryPrecision:    TagColorOrange,
	TagCategoryArchitecture: TagColorRed,
	TagCategoryTraining:     TagColorYellow,
	TagCategoryOther:        TagColorGray,
}

// providerTags contains tags for model providers/vendors
var providerTags = map[string]bool{
	"openai":      true,
	"anthropic":   true,
	"google":      true,
	"meta":        true,
	"facebook":    true, // Meta's legacy name on HuggingFace
	"microsoft":   true,
	"alibaba":     true,
	"bytedance":   true,
	"tencent":     true,
	"baidu":       true,
	"deepseek":    true,
	"qwen":        true,
	"llama":       true,
	"mistral":     true,
	"cohere":      true,
	"xai":         true,
	"yi":          true,
	"minimax":     true,
	"internlm":    true,
	"sensetime":   true,
	"huawei":      true,
	"zhipu":       true, // GLM series
	"chatglm":     true,
	"glm":         true,
	"gemma":       true, // Google's Gemma
	"phi":         true, // Microsoft's Phi
	"falcon":      true, // TII
	"starcoder":   true, // BigCode
	"codellama":   true, // Meta's Code Llama
	"vicuna":      true,
	"wizardlm":    true,
	"openchat":    true,
	"neural-chat": true,
	"custom":      true,
	"internal":    true,
}

// taskModalityTags contains tags for model task types and modalities
var taskModalityTags = map[string]bool{
	// Text tasks
	"text-generation":       true,
	"chat":                  true,
	"instruction-following": true,
	"code-generation":       true,
	"reasoning":             true,
	"rag":                   true,
	"conversational":        true,
	"text2text-generation":  true,
	"question-answering":    true,
	"summarization":         true,
	"translation":           true,
	"fill-mask":             true,
	"text-classification":   true,
	"token-classification":  true,
	"sentence-similarity":   true,
	"feature-extraction":    true,
	// Multimodal tasks
	"vision-language":              true,
	"image-understanding":          true,
	"image-generation":             true,
	"video-understanding":          true,
	"audio-understanding":          true,
	"speech-recognition":           true,
	"speech-synthesis":             true,
	"image-to-text":                true,
	"text-to-image":                true,
	"text-to-video":                true,
	"text-to-audio":                true,
	"text-to-speech":               true,
	"automatic-speech-recognition": true,
	"image-classification":         true,
	"object-detection":             true,
	"image-segmentation":           true,
	"video-classification":         true,
	"visual-question-answering":    true,
	"document-question-answering":  true,
	"image-text-to-text":           true,
	// Special tasks
	"agent":                    true,
	"embedding":                true,
	"embeddings":               true,
	"classifier":               true,
	"rl":                       true,
	"rlhf":                     true,
	"tokenizer":                true,
	"zero-shot-classification": true,
}

// modelSizeTags contains tags for model parameter sizes
var modelSizeTags = map[string]bool{
	"1b":    true,
	"2b":    true,
	"3b":    true,
	"4b":    true,
	"7b":    true,
	"8b":    true,
	"13b":   true,
	"14b":   true,
	"32b":   true,
	"34b":   true,
	"70b":   true,
	"72b":   true,
	"110b":  true,
	"400b":  true,
	"500b":  true,
	"500b+": true,
	// Also match variations with decimals
	"0.5b": true,
	"1.5b": true,
	"1.8b": true,
	"2.7b": true,
	"6.7b": true,
	"7.1b": true,
}

// precisionQuantizationTags contains tags for model precision and quantization methods
var precisionQuantizationTags = map[string]bool{
	// Standard precision
	"fp32": true,
	"fp16": true,
	"bf16": true,
	"fp8":  true,
	"int8": true,
	"int4": true,
	"nf4":  true,
	// GGUF/GGML quantization formats
	"q4_k_m": true,
	"q4_k_s": true,
	"q5_k_m": true,
	"q5_k_s": true,
	"q6_k":   true,
	"q8_0":   true,
	"q2_k":   true,
	"q3_k_m": true,
	"q3_k_s": true,
	"q3_k_l": true,
	"q4_0":   true,
	"q4_1":   true,
	"q5_0":   true,
	"q5_1":   true,
	// GPTQ formats
	"gptq":      true,
	"gptq-4bit": true,
	"gptq-8bit": true,
	// AWQ formats
	"awq":      true,
	"awq-4bit": true,
	// Ascend (Huawei NPU) specific
	"fp16-acl": true,
	"bf16-acl": true,
	"8bit-a2":  true,
	// Quantization method tags
	"4bit":         true,
	"8bit":         true,
	"quantized":    true,
	"gguf":         true,
	"ggml":         true,
	"exl2":         true,
	"bitsandbytes": true,
}

// architectureTags contains tags for model architectures
var architectureTags = map[string]bool{
	// Transformer variants
	"transformer":        true,
	"decoder-only":       true,
	"encoder-only":       true,
	"encoder-decoder":    true,
	"mixture-of-experts": true,
	"moe":                true,
	"sparse-moe":         true,
	// Alternative architectures
	"rwkv":        true,
	"mamba":       true,
	"ssm":         true,
	"state-space": true,
	// Vision architectures
	"swin":               true,
	"vit":                true,
	"vision-transformer": true,
	"clip":               true,
	"dino":               true,
	// Audio architectures
	"conformer":            true,
	"whisper":              true,
	"whisper-architecture": true,
	"wav2vec":              true,
	// High-level model types
	"llm":              true,
	"vlm":              true,
	"diffusion":        true,
	"stable-diffusion": true,
	"gan":              true,
	"autoregressive":   true,
	"bert":             true,
	"gpt":              true,
	"t5":               true,
	"bart":             true,
	"roberta":          true,
	"xlnet":            true,
	"electra":          true,
}

// trainingMethodTags contains tags for training methods and data sources
var trainingMethodTags = map[string]bool{
	"rlhf":        true,
	"sft":         true,
	"dpo":         true,
	"ppo":         true,
	"aligned":     true,
	"unaligned":   true,
	"pretrained":  true,
	"fine-tuned":  true,
	"finetuned":   true,
	"instruct":    true,
	"instruction": true,
	"chat-tuned":  true,
	"base":        true,
	"lora":        true,
	"qlora":       true,
	"adapter":     true,
	"merged":      true,
}

// modelSizePattern matches model size patterns like "7b", "70B", "1.5B", etc.
var modelSizePattern = regexp.MustCompile(`(?i)^(\d+\.?\d*)(b|m)$`)

// filterModelTags processes input tags and returns them with color information.
// If includeUnmatched is true, unmatched tags are included with gray color.
// If includeUnmatched is false, only tags matching the 6 categories are returned.
func filterModelTags(tags []string, includeUnmatched bool) []TagWithCategory {
	if len(tags) == 0 {
		return []TagWithCategory{}
	}

	result := make([]TagWithCategory, 0, len(tags))
	seen := make(map[string]bool)

	for _, tag := range tags {
		// Normalize tag for matching (lowercase, trim spaces)
		normalizedTag := strings.ToLower(strings.TrimSpace(tag))
		if normalizedTag == "" {
			continue
		}

		// Skip if we've already added this tag (avoid duplicates)
		if seen[normalizedTag] {
			continue
		}

		// Get the category for this tag and map to color
		category := getTagCategory(normalizedTag)
		var color TagColor
		if category != "" {
			color = categoryColorMap[category]
		} else {
			// Unmatched tags
			if !includeUnmatched {
				// Skip unmatched tags for local mode
				continue
			}
			color = TagColorGray
		}

		result = append(result, TagWithCategory{
			Value: tag, // Keep original case
			Color: color,
		})
		seen[normalizedTag] = true
	}

	return result
}

// getTagCategory determines which category a tag belongs to.
// Returns empty string if tag doesn't match any category.
func getTagCategory(tag string) TagCategory {
	// Check exact matches first (order matters for priority)

	// 1. Provider (exact match)
	if providerTags[tag] {
		return TagCategoryProvider
	}

	// 2. Task/Modality (exact match)
	if taskModalityTags[tag] {
		return TagCategoryTask
	}

	// 3. Model Size (exact match)
	if modelSizeTags[tag] {
		return TagCategorySize
	}

	// 4. Precision/Quantization (exact match)
	if precisionQuantizationTags[tag] {
		return TagCategoryPrecision
	}

	// 5. Architecture (exact match)
	if architectureTags[tag] {
		return TagCategoryArchitecture
	}

	// 6. Training Method (exact match)
	if trainingMethodTags[tag] {
		return TagCategoryTraining
	}

	// Check pattern matches

	// Model size pattern (e.g., "7b", "13B", "1.5b")
	if modelSizePattern.MatchString(tag) {
		return TagCategorySize
	}

	// Quantization pattern (e.g., "q4_k_m", "q8_0")
	if strings.HasPrefix(tag, "q") && (strings.Contains(tag, "_") || len(tag) <= 5) {
		if matched, _ := regexp.MatchString(`^q\d+`, tag); matched {
			return TagCategoryPrecision
		}
	}

	// Provider prefix match (e.g., "llama-3" matches "llama", "qwen2" matches "qwen")
	if matchesProviderPrefix(tag) {
		return TagCategoryProvider
	}

	// Architecture prefix match (e.g., "bert-base" matches "bert", "gpt-4" matches "gpt")
	if matchesArchitecturePrefix(tag) {
		return TagCategoryArchitecture
	}

	return "" // No match
}

// extractTagValues extracts plain string values from categorized tags.
// Used for backward compatible storage.
func extractTagValues(tags []TagWithCategory) []string {
	if len(tags) == 0 {
		return []string{}
	}
	result := make([]string, len(tags))
	for i, tag := range tags {
		result[i] = tag.Value
	}
	return result
}

// CategorizeTags converts a plain string tag array to categorized tags with color info.
// includeUnmatched: if true, include unmatched tags with gray color (for remote mode)
//
//	if false, only include tags matching the 6 categories (for local mode)
func CategorizeTags(tags []string, includeUnmatched bool) []TagWithCategory {
	return filterModelTags(tags, includeUnmatched)
}

// CategorizeTagString converts a comma-separated tag string to categorized tags.
// includeUnmatched: if true, include unmatched tags with gray color (for remote mode)
//
//	if false, only include tags matching the 6 categories (for local mode)
func CategorizeTagString(tagsStr string, includeUnmatched bool) []TagWithCategory {
	if tagsStr == "" {
		return []TagWithCategory{}
	}
	tags := strings.Split(tagsStr, ",")
	// Trim whitespace from each tag
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}
	return filterModelTags(tags, includeUnmatched)
}

// matchesProviderPrefix checks if tag starts with any known provider name
// This handles cases like "llama-3", "qwen2", "qwen2.5", "mistral-7b-instruct"
func matchesProviderPrefix(tag string) bool {
	for provider := range providerTags {
		if strings.HasPrefix(tag, provider) && len(tag) > len(provider) {
			// Ensure the match is at a word boundary (followed by digit, hyphen, or underscore)
			nextChar := tag[len(provider)]
			if nextChar == '-' || nextChar == '_' || nextChar == '.' || (nextChar >= '0' && nextChar <= '9') {
				return true
			}
		}
	}
	return false
}

// matchesArchitecturePrefix checks if tag starts with any known architecture name
// This handles cases like "bert-base", "gpt-4", "t5-large", "whisper-large-v3"
func matchesArchitecturePrefix(tag string) bool {
	for arch := range architectureTags {
		if strings.HasPrefix(tag, arch) && len(tag) > len(arch) {
			// Ensure the match is at a word boundary (followed by digit, hyphen, or underscore)
			nextChar := tag[len(arch)]
			if nextChar == '-' || nextChar == '_' || nextChar == '.' || (nextChar >= '0' && nextChar <= '9') {
				return true
			}
		}
	}
	return false
}

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
	DisplayName     string
	Description     string
	Icon            string
	Label           string
	Tags            []string          // Plain tags for storage (backward compatible)
	CategorizedTags []TagWithCategory // Tags with category and color information for frontend display
	MaxTokens       int               // Maximum context length from config.json (max_position_embeddings)
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
	// Filter tags to only include those matching our classification categories (local mode):
	// Provider, Task/Modality, Model Size, Precision/Quantization, Architecture, Training Method
	// For local mode, we exclude unmatched tags (gray) during storage
	categorizedTags := filterModelTags(meta.Tags, false) // false = exclude unmatched (local mode)
	info.CategorizedTags = categorizedTags
	info.Tags = extractTagValues(categorizedTags) // Plain string array for storage
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
