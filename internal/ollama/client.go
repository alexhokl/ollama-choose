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

// CriteriaMap is a string map that preserves insertion order when marshaled
// to JSON. System One breaks choice ties by option order, so criteria must
// keep the order options were given instead of Go's alphabetical map sort.
type CriteriaMap struct {
	keys []string
	vals map[string]string
}

// NewCriteriaMap returns an empty ordered criteria map.
func NewCriteriaMap() *CriteriaMap {
	return &CriteriaMap{vals: make(map[string]string)}
}

// Set adds a key, or replaces its value while keeping its original position.
func (m *CriteriaMap) Set(key, value string) {
	if !m.Has(key) {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = value
}

// Has reports whether the key exists.
func (m *CriteriaMap) Has(key string) bool {
	_, ok := m.vals[key]
	return ok
}

// Len returns the number of entries.
func (m *CriteriaMap) Len() int {
	return len(m.keys)
}

// Keys returns the keys in insertion order.
func (m *CriteriaMap) Keys() []string {
	return append([]string(nil), m.keys...)
}

// Get returns the description for a key.
func (m *CriteriaMap) Get(key string) string {
	return m.vals[key]
}

// MarshalJSON encodes the map as a JSON object in insertion order.
func (m *CriteriaMap) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		valJSON, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(keyJSON)
		b.WriteByte(':')
		b.Write(valJSON)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// SystemOneQuestion describes one typed question in a System One request.
// Type "noul" needs only instructions. "choice" also requires Criteria as
// a *CriteriaMap and "score" as a []string of ordered level descriptions.
type SystemOneQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
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

// SystemOneAnswer is the typed answer to one question. The Type field
// discriminates which fields carry data: noul answers fill Noul, choice
// answers fill Choice/Probabilities/Confidence, and score answers fill
// Score/Legend/Probabilities/Confidence.
type SystemOneAnswer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul"`
	Choice        string             `json:"choice"`
	Score         float64            `json:"score"`
	Probabilities map[string]float64 `json:"probabilities"`
	Legend        map[string]string  `json:"legend"`
	Confidence    float64            `json:"confidence"`
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
