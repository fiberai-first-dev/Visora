package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

const (
	modelGeminiLite = "google/gemini-2.5-flash-lite"
	modelQwenFlash  = "alibaba/qwen3.5-flash"
	modelGPTMini    = "openai/gpt-4o-mini"
)

// GetModel returns the unified model for brand, intents, gaps, recommendations, fixes, and GEO.
func GetModel() string {
	return CanonicalModel(os.Getenv("VISORA_LLM_MODEL"))
}

// CanonicalModel remaps gated/broken gateway IDs onto cheap working ones.
func CanonicalModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case m == "":
		return modelGeminiLite
	case strings.Contains(m, "deepseek-v4"), strings.Contains(m, "deepseek/deepseek-v4"):
		return modelGeminiLite
	case strings.Contains(m, "ling-3.0"), strings.Contains(m, "sante-free"):
		return modelQwenFlash
	case strings.Contains(m, "poolside"), strings.Contains(m, "laguna"):
		return modelQwenFlash
	default:
		return strings.TrimSpace(model)
	}
}

func fallbackModels(primary string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(m string) {
		m = CanonicalModel(m)
		if m == "" || seen[m] {
			return
		}
		seen[m] = true
		out = append(out, m)
	}
	add(primary)
	add(modelGeminiLite)
	add(modelQwenFlash)
	add(modelGPTMini)
	return out
}

// GetGeoModel returns the model for GEO queries (defaults to the same cheap gateway model).
func GetGeoModel() string {
	model := os.Getenv("GEO_OPENAI_MODEL")
	if model == "" {
		return GetModel()
	}
	return model
}

func apiBaseURL() string {
	base := strings.TrimRight(os.Getenv("OPENAI_BASE_URL"), "/")
	if base == "" {
		return "https://ai-gateway.vercel.sh/v1"
	}
	return base
}

// zaiThinkingNeeded is true only for Z.AI flash models that otherwise dump into reasoning_content.
func zaiThinkingNeeded(base string) bool {
	return strings.Contains(strings.ToLower(base), "z.ai")
}

// NewClient builds an OpenAI-compatible client (Vercel AI Gateway / Z.AI / OpenAI).
func NewClient() (*openai.Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = apiBaseURL()
	return openai.NewClientWithConfig(config), nil
}

// NewGeoClient builds the GEO client — same gateway key/base as core AI by default.
func NewGeoClient() (*openai.Client, error) {
	apiKey := os.Getenv("GEO_OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GEO_OPENAI_API_KEY not set")
	}
	config := openai.DefaultConfig(apiKey)
	if base := os.Getenv("GEO_OPENAI_BASE_URL"); base != "" {
		config.BaseURL = strings.TrimRight(base, "/")
	} else {
		config.BaseURL = apiBaseURL()
	}
	return openai.NewClientWithConfig(config), nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Thinking    *struct {
		Type string `json:"type"`
	} `json:"thinking,omitempty"`
	ResponseFormat map[string]interface{} `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Complete calls the configured OpenAI-compatible API (Vercel AI Gateway by default).
func Complete(ctx context.Context, model string, messages []openai.ChatCompletionMessage, temperature float32, maxTokens int) (string, error) {
	var lastErr error
	for _, try := range fallbackModels(model) {
		text, err := completeOnce(ctx, try, messages, temperature, maxTokens)
		if err == nil {
			return text, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("empty LLM response")
}

func completeOnce(ctx context.Context, model string, messages []openai.ChatCompletionMessage, temperature float32, maxTokens int) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	base := apiBaseURL()
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY not set")
	}
	if model == "" {
		model = GetModel()
	}

	payload := chatRequest{
		Model:       model,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}
	if zaiThinkingNeeded(base) {
		payload.Thinking = &struct{ Type string `json:"type"` }{Type: "disabled"}
	}
	for _, m := range messages {
		payload.Messages = append(payload.Messages, chatMessage{Role: m.Role, Content: m.Content})
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en-US,en")

	client := &http.Client{Timeout: 90 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode chat response: %w (%s)", err, truncate(string(raw), 200))
	}
	if res.StatusCode >= 400 {
		msg := string(raw)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return "", fmt.Errorf("chat completion %d: %s", res.StatusCode, truncate(msg, 300))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("empty LLM response")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		text = strings.TrimSpace(parsed.Choices[0].Message.ReasoningContent)
	}
	if text == "" {
		return "", fmt.Errorf("empty LLM content from %s (try a non-reasoning model)", model)
	}
	return text, nil
}

// CompleteJSON is Complete + JSON-object bias for structured outputs.
func CompleteJSON(ctx context.Context, model string, messages []openai.ChatCompletionMessage, temperature float32) (string, error) {
	if model == "" {
		model = GetModel()
	}
	models := fallbackModels(model)

	var lastErr error
	for _, try := range models {
		text, err := completeJSONOnce(ctx, try, messages, temperature)
		if err == nil && strings.Contains(text, "{") {
			return ExtractJSONObject(text), nil
		}
		if err != nil {
			lastErr = err
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "403") || strings.Contains(msg, "404") || strings.Contains(msg, "forbidden") || strings.Contains(msg, "not found") {
				continue
			}
		}
	}

	// Last resort: plain completion, then pull the first JSON object out.
	for _, try := range models {
		text, err := Complete(ctx, try, messages, temperature, 4096)
		if err != nil {
			lastErr = err
			continue
		}
		extracted := ExtractJSONObject(text)
		if strings.Contains(extracted, "{") {
			return extracted, nil
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("empty JSON from %s", model)
}

func completeJSONOnce(ctx context.Context, model string, messages []openai.ChatCompletionMessage, temperature float32) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GEO_OPENAI_API_KEY")
	}
	base := apiBaseURL()
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY not set")
	}

	payload := chatRequest{
		Model:       model,
		Temperature: temperature,
		MaxTokens:   8192,
		ResponseFormat: map[string]interface{}{
			"type": "json_object",
		},
	}
	if zaiThinkingNeeded(base) {
		payload.Thinking = &struct{ Type string `json:"type"` }{Type: "disabled"}
	}
	for _, m := range messages {
		payload.Messages = append(payload.Messages, chatMessage{Role: m.Role, Content: m.Content})
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en-US,en")

	client := &http.Client{Timeout: 90 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}
	if res.StatusCode >= 400 {
		payload.ResponseFormat = nil
		body2, _ := json.Marshal(payload)
		req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body2))
		req2.Header.Set("Authorization", "Bearer "+apiKey)
		req2.Header.Set("Content-Type", "application/json")
		res2, err2 := client.Do(req2)
		if err2 != nil {
			return "", fmt.Errorf("chat completion %d: %s", res.StatusCode, truncate(string(raw), 300))
		}
		defer res2.Body.Close()
		raw2, _ := io.ReadAll(res2.Body)
		if res2.StatusCode >= 400 {
			return "", fmt.Errorf("chat completion %d: %s", res2.StatusCode, truncate(string(raw2), 300))
		}
		if err := json.Unmarshal(raw2, &parsed); err != nil {
			return "", fmt.Errorf("decode chat response: %w", err)
		}
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("empty LLM response")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		text = strings.TrimSpace(parsed.Choices[0].Message.ReasoningContent)
	}
	if text == "" {
		return "", fmt.Errorf("empty LLM content")
	}
	return text, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ExtractJSONObject pulls the first complete JSON object from an LLM response that may
// include markdown fences, thinking tags, trailing instructions, or preamble text.
func ExtractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```JSON")
		raw = strings.TrimPrefix(raw, "```")
		if idx := strings.LastIndex(raw, "```"); idx >= 0 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
	}
	start := strings.Index(raw, "{")
	if start < 0 {
		return raw
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(raw[start : i+1])
			}
		}
	}
	end := strings.LastIndex(raw, "}")
	if end > start {
		return strings.TrimSpace(raw[start : end+1])
	}
	return raw
}
