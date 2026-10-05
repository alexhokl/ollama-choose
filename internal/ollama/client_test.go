package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestCriteriaMap(t *testing.T) {
	m := NewCriteriaMap()
	m.Set("billing", "Payments and refunds")
	m.Set("technical", "Bugs")
	m.Set("other", "None of the above")

	if m.Len() != 3 {
		t.Errorf("Len() = %d, want 3", m.Len())
	}
	if got, want := m.Keys(), []string{"billing", "technical", "other"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
	if m.Get("billing") != "Payments and refunds" {
		t.Errorf("Get(billing) = %q", m.Get("billing"))
	}
	if !m.Has("technical") || m.Has("missing") {
		t.Error("Has() disagrees with contents")
	}

	// Overwriting keeps the original position.
	m.Set("technical", "Bugs and integrations")
	if m.Len() != 3 {
		t.Errorf("Len() = %d after overwrite, want 3", m.Len())
	}
	if got, want := m.Keys(), []string{"billing", "technical", "other"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v after overwrite, want %v", got, want)
	}

	data, err := m.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	want := `{"billing":"Payments and refunds","technical":"Bugs and integrations","other":"None of the above"}`
	if string(data) != want {
		t.Errorf("MarshalJSON() = %s, want %s", data, want)
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

func TestSystemOneAllAnswerTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"model": "clef",
			"answers": {
				"team": {
					"type": "choice",
					"choice": "billing",
					"probabilities": {"billing": 0.981, "technical": 0.013, "other": 0.006},
					"confidence": 0.924
				},
				"refund": {"type": "noul", "noul": 0.996},
				"urgency": {
					"type": "score",
					"score": 0.704,
					"legend": {"0": "Routine", "1": "Soon", "2": "Urgent"},
					"probabilities": {"0": 0.451, "1": 0.353, "2": 0.196},
					"confidence": 0.071
				}
			},
			"usage": {"input_tokens": 1204, "output_tokens": 3}
		}`))
	}))
	t.Cleanup(srv.Close)

	choice := NewCriteriaMap()
	choice.Set("billing", "Payments and refunds")
	choice.Set("technical", "Bugs and integrations")
	choice.Set("other", "None of the above")

	client := NewClient(srv.URL, 5*time.Second)
	resp, err := client.SystemOne(context.Background(), SystemOneRequest{
		Model: "clef",
		State: "I was charged twice. Please refund the extra payment.",
		Questions: map[string]SystemOneQuestion{
			"team":    {Type: "choice", Instructions: "Which team should handle this ticket?", Criteria: choice},
			"refund":  {Type: "noul", Instructions: "Does the customer explicitly ask for a refund?"},
			"urgency": {Type: "score", Instructions: "How urgent is this ticket?", Criteria: []string{"Routine", "Soon", "Urgent"}},
		},
	})
	if err != nil {
		t.Fatalf("SystemOne() error = %v", err)
	}

	team, ok := resp.Answers["team"]
	if !ok {
		t.Fatal("team answer missing")
	}
	if team.Type != "choice" || team.Choice != "billing" || team.Confidence != 0.924 {
		t.Errorf("team = %+v", team)
	}
	wantProbs := map[string]float64{"billing": 0.981, "technical": 0.013, "other": 0.006}
	if !reflect.DeepEqual(team.Probabilities, wantProbs) {
		t.Errorf("team probabilities = %v, want %v", team.Probabilities, wantProbs)
	}

	refund, ok := resp.Answers["refund"]
	if !ok {
		t.Fatal("refund answer missing")
	}
	if refund.Type != "noul" || refund.Noul != 0.996 {
		t.Errorf("refund = %+v", refund)
	}

	urgency, ok := resp.Answers["urgency"]
	if !ok {
		t.Fatal("urgency answer missing")
	}
	if urgency.Type != "score" || urgency.Score != 0.704 || urgency.Confidence != 0.071 {
		t.Errorf("urgency = %+v", urgency)
	}
	wantLegend := map[string]string{"0": "Routine", "1": "Soon", "2": "Urgent"}
	if !reflect.DeepEqual(urgency.Legend, wantLegend) {
		t.Errorf("urgency legend = %v, want %v", urgency.Legend, wantLegend)
	}
	wantScoreProbs := map[string]float64{"0": 0.451, "1": 0.353, "2": 0.196}
	if !reflect.DeepEqual(urgency.Probabilities, wantScoreProbs) {
		t.Errorf("urgency probabilities = %v, want %v", urgency.Probabilities, wantScoreProbs)
	}

	if resp.Usage.InputTokens != 1204 || resp.Usage.OutputTokens != 3 {
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
