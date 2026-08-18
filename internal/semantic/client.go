package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/DheerG/prosecheck/internal/checker"
)

type Options struct {
	Endpoint string
	Model    string
	Timeout  time.Duration
}

type Client struct {
	endpoint string
	model    string
	http     *http.Client
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type modelReview struct {
	Summary  string         `json:"summary"`
	Findings []modelFinding `json:"findings"`
}

type modelFinding struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

var allowedCodes = map[string]bool{
	"SEM001": true,
	"SEM002": true,
	"SEM003": true,
	"SEM004": true,
	"SEM005": true,
	"SEM006": true,
}

var findingLine = regexp.MustCompile(`^(?:[-*]\s*)?(SEM00[1-6])\s*\|\s*(.+?)\s*\|\s*(.+)$`)

func NewClient(options Options) *Client {
	return &Client{
		endpoint: chatEndpoint(options.Endpoint),
		model:    options.Model,
		http:     &http.Client{Timeout: options.Timeout},
	}
}

func (c *Client) Review(ctx context.Context, message, diff string) ([]checker.Finding, error) {
	request := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt(message, diff)},
		},
		Temperature: 0,
		MaxTokens:   600,
	}

	content, _, err := c.send(ctx, request)
	if err != nil {
		return nil, err
	}
	return parseReview(content)
}

func parseReview(content string) ([]checker.Finding, error) {
	content = stripThinking(content)
	if strings.EqualFold(strings.TrimSpace(content), "clear") {
		return []checker.Finding{}, nil
	}

	findings := make([]checker.Finding, 0, 3)
	for _, line := range strings.Split(content, "\n") {
		matches := findingLine.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) != 4 {
			continue
		}
		item := modelFinding{Code: matches[1], Message: strings.TrimSpace(matches[2]), Suggestion: strings.TrimSpace(matches[3])}
		if copiedPlaceholder(item) {
			continue
		}
		findings = append(findings, checker.Finding{
			Code: item.Code, Severity: checker.SeverityInfo, Source: checker.SourceSemantic,
			Message: item.Message, Suggestion: item.Suggestion,
		})
	}
	if len(findings) > 0 {
		return findings, nil
	}

	var review modelReview
	object, err := extractJSONObject(content)
	if err != nil {
		return nil, errors.New("the model review did not use the required CLEAR or CODE | message | suggestion format")
	}
	if err := json.Unmarshal([]byte(object), &review); err != nil {
		return nil, fmt.Errorf("the model returned invalid JSON: %w", err)
	}

	jsonFindings := make([]checker.Finding, 0, len(review.Findings))
	for _, item := range review.Findings {
		item.Code = strings.ToUpper(strings.TrimSpace(item.Code))
		item.Message = strings.TrimSpace(item.Message)
		item.Suggestion = strings.TrimSpace(item.Suggestion)
		if !allowedCodes[item.Code] || item.Message == "" || copiedPlaceholder(item) {
			continue
		}
		jsonFindings = append(jsonFindings, checker.Finding{
			Code:       item.Code,
			Severity:   checker.SeverityInfo,
			Source:     checker.SourceSemantic,
			Message:    item.Message,
			Suggestion: item.Suggestion,
		})
	}
	return jsonFindings, nil
}

func stripThinking(content string) string {
	for {
		start := strings.Index(content, "<think>")
		end := strings.Index(content, "</think>")
		if start < 0 || end < start {
			return strings.TrimSpace(content)
		}
		content = content[:start] + content[end+len("</think>"):]
	}
}

func copiedPlaceholder(item modelFinding) bool {
	return strings.EqualFold(item.Message, "clear problem") ||
		strings.EqualFold(item.Suggestion, "specific correction")
}

func (c *Client) send(ctx context.Context, payload chatRequest) (string, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return "", 0, fmt.Errorf("cannot reach %s: %w", c.endpoint, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", response.StatusCode, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", response.StatusCode, fmt.Errorf("the model server returned HTTP %d: %s", response.StatusCode, compact(responseBody))
	}
	var decoded chatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", response.StatusCode, fmt.Errorf("the model server returned invalid JSON: %w", err)
	}
	if decoded.Error != nil {
		return "", response.StatusCode, errors.New(decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == "" {
		return "", response.StatusCode, errors.New("the model server returned no review")
	}
	return decoded.Choices[0].Message.Content, response.StatusCode, nil
}

func chatEndpoint(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(trimmed)
	if err == nil && strings.HasSuffix(parsed.Path, "/chat/completions") {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/chat/completions"
	}
	return trimmed + "/v1/chat/completions"
}

func compact(value []byte) string {
	text := strings.Join(strings.Fields(string(value)), " ")
	if len(text) > 300 {
		return text[:300] + "…"
	}
	return text
}

func extractJSONObject(content string) (string, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	for start := 0; start < len(content); start++ {
		if content[start] != '{' {
			continue
		}
		depth := 0
		inString := false
		escaped := false
		for end := start; end < len(content); end++ {
			character := content[end]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if character == '\\' {
					escaped = true
				} else if character == '"' {
					inString = false
				}
				continue
			}
			switch character {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := content[start : end+1]
					if json.Valid([]byte(candidate)) {
						return candidate, nil
					}
					break
				}
			}
		}
	}
	return "", errors.New("the model response did not contain a JSON object")
}
