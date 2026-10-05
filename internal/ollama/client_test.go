package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:11434/", time.Second)
	if c.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", c.BaseURL)
	}
	if c.HTTPClient.Timeout != time.Second {
		t.Errorf("Timeout = %v, want %v", c.HTTPClient.Timeout, time.Second)
	}
}

func TestSystemOne(t *testing.T) {
	var req struct {
		Model     string                       `json:"model"`
		State     string                       `json:"state"`
		Images    []string                     `json:"images"`
		Questions map[string]SystemOneQuestion `json:"questions"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path = %q, want /v1/systemone", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{
			"model": "clef-flash",
			"answers": {"statement": {"type": "noul", "noul": 0.9}},
			"usage": {"input_tokens": 151, "output_tokens": 0}
		}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, 5*time.Second)
	resp, err := client.SystemOne(context.Background(), SystemOneRequest{
		Model:  "clef-flash",
		State:  "2+2 equals 4.",
		Images: []string{"aGVsbG8="},
		Questions: map[string]SystemOneQuestion{
			"statement": {Type: "noul", Instructions: "Is the state factually correct?"},
		},
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v", err)
	}
	if req.Model != "clef-flash" {
		t.Errorf("model = %q", req.Model)
	}
	if req.State != "2+2 equals 4." {
		t.Errorf("state = %q", req.State)
	}
	if len(req.Images) != 1 || req.Images[0] != "aGVsbG8=" {
		t.Errorf("images = %v, want [aGVsbG8=]", req.Images)
	}
	q, ok := req.Questions["statement"]
	if !ok || q.Type != "noul" || q.Instructions != "Is the state factually correct?" {
		t.Errorf("questions = %+v", req.Questions)
	}
	answer, ok := resp.Answers["statement"]
	if !ok {
		t.Fatal("answer for statement missing")
	}
	if answer.Type != "noul" || answer.Noul != 0.9 {
		t.Errorf("answer = %+v, want noul 0.9", answer)
	}
	if resp.Usage.InputTokens != 151 || resp.Usage.OutputTokens != 0 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestSystemOneServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model 'x' not found, try pulling it first"}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, 5*time.Second)
	_, err := client.SystemOne(context.Background(), SystemOneRequest{
		Model:     "x",
		State:     "state",
		Questions: map[string]SystemOneQuestion{"q": {Type: "noul", Instructions: "i"}},
	})
	if err == nil || !strings.Contains(err.Error(), "model 'x' not found") {
		t.Errorf("SystemOne() error = %v, want Ollama error surfaced", err)
	}
}

func TestSystemOneMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, 5*time.Second)
	if _, err := client.SystemOne(context.Background(), SystemOneRequest{
		Model:     "m",
		State:     "state",
		Questions: map[string]SystemOneQuestion{"q": {Type: "noul", Instructions: "i"}},
	}); err == nil {
		t.Error("SystemOne() want error for malformed response")
	}
}
