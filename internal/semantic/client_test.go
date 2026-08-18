package semantic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DheerG/prosecheck/internal/checker"
)

func TestReviewUsesLineProtocol(t *testing.T) {
	client := testClient(t, func(request *http.Request) *http.Response {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		var payload chatRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "bonsai" {
			t.Errorf("unexpected model %q", payload.Model)
		}
		if payload.ResponseFormat != nil {
			t.Errorf("expected unconstrained output, got %#v", payload.ResponseFormat)
		}
		content := "SEM002 | The reason is missing. | Explain the operational constraint."
		body, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": content}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return httpResponse(http.StatusOK, string(body))
	})

	findings, err := client.Review(context.Background(), "Change retry handling", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Code != "SEM002" || findings[0].Source != checker.SourceSemantic || findings[0].Severity != checker.SeverityInfo {
		t.Fatalf("unexpected findings: %#v", findings)
	}
}

func TestReviewAcceptsClearResult(t *testing.T) {
	var requests atomic.Int32
	client := testClient(t, func(request *http.Request) *http.Response {
		requests.Add(1)
		var payload chatRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return httpResponse(http.StatusOK, `{"choices":[{"message":{"content":"CLEAR"}}]}`)
	})

	findings, err := client.Review(context.Background(), "Preserve job state during shutdown", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 || requests.Load() != 1 {
		t.Fatalf("expected a successful review, got %#v after %d requests", findings, requests.Load())
	}
}

func TestReviewIgnoresUnknownCodes(t *testing.T) {
	client := testClient(t, func(request *http.Request) *http.Response {
		return httpResponse(http.StatusOK, `{"choices":[{"message":{"content":"{\"summary\":\"Review\",\"findings\":[{\"code\":\"SEM999\",\"message\":\"Invented rule\",\"suggestion\":\"None\"}]}"}}]}`)
	})

	findings, err := client.Review(context.Background(), "Keep user names in audit records", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected unknown codes to be ignored, got %#v", findings)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func testClient(t *testing.T, handler func(*http.Request) *http.Response) *Client {
	t.Helper()
	client := NewClient(Options{Endpoint: "http://model.test/v1", Model: "bonsai", Timeout: time.Second})
	client.http.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return handler(request), nil
	})
	return client
}

func httpResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestExtractJSONObjectHandlesThinkingText(t *testing.T) {
	content := `<think>I will review the message.</think>
{"summary":"Clear","findings":[]}`
	object, err := extractJSONObject(content)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(object, `"findings":[]`) {
		t.Fatalf("unexpected object %q", object)
	}
}
