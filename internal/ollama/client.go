// Package ollama provides a minimal client for the Ollama HTTP API.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxResponseBytes caps how much of an Ollama response is read.
const maxResponseBytes = 16 << 20 // 16 MiB

// SystemOneQuestion describes one typed question in a System One request.
// Type "noul" requires only instructions; "choice" and "score" questions
// add criteria fields when the client grows to support them.
type SystemOneQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

// SystemOneRequest describes a /v1/systemone call (Ollama v0.35+).
// State must be a nonempty string, and only GGUF decision models
// such as clef and clef-flash are supported by the server.
type SystemOneRequest struct {
	Model     string                       `json:"model"`
	State     string                       `json:"state"`
	Images    []string                     `json:"images,omitempty"` // base64-encoded images
	Questions map[string]SystemOneQuestion `json:"questions"`
}

// SystemOneAnswer is the typed answer to one question. For noul questions,
// Noul holds the probability of true, from 0 to 1.
type SystemOneAnswer struct {
	Type string  `json:"type"`
	Noul float64 `json:"noul"`
}

// SystemOneResponse holds the answers and token usage for a System One call.
type SystemOneResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]SystemOneAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Client talks to an Ollama server over HTTP.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient returns a Client for the Ollama server at baseURL
// (for example "http://localhost:11434"), using the given per-request timeout.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// SystemOne sends req to the server's /v1/systemone endpoint and returns
// the answers and usage for all questions.
func (c *Client) SystemOne(ctx context.Context, req SystemOneRequest) (*SystemOneResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call Ollama: %w", err)
	}
	defer httpResp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if json.Unmarshal(data, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("ollama: %s", errResp.Error)
		}
		return nil, fmt.Errorf("ollama: unexpected status %s", httpResp.Status)
	}

	var resp SystemOneResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}
