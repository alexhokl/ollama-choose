package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func resetChoiceFlags() {
	choiceFlags = choiceOptions{}
}

func TestParseChoiceOptions(t *testing.T) {
	tooMany := make([]string, 27)
	for i := range tooMany {
		tooMany[i] = "opt" + strconv.Itoa(i)
	}

	tests := []struct {
		name     string
		raw      []string
		wantKeys []string
		wantDesc map[string]string
		wantErr  string
	}{
		{
			name:     "keys without descriptions",
			raw:      []string{"meals", "travel"},
			wantKeys: []string{"meals", "travel"},
			wantDesc: map[string]string{"meals": "meals", "travel": "travel"},
		},
		{
			name:     "keys with descriptions",
			raw:      []string{"billing: Payments and refunds", "technical: Bugs and integrations"},
			wantKeys: []string{"billing", "technical"},
			wantDesc: map[string]string{"billing": "Payments and refunds", "technical": "Bugs and integrations"},
		},
		{
			name:     "description containing colon",
			raw:      []string{"a: one: two", "b"},
			wantKeys: []string{"a", "b"},
			wantDesc: map[string]string{"a": "one: two", "b": "b"},
		},
		{
			name:    "one option",
			raw:     []string{"a"},
			wantErr: "2 to 26 options are required",
		},
		{
			name:    "too many options",
			raw:     tooMany,
			wantErr: "2 to 26 options are required",
		},
		{
			name:    "blank key",
			raw:     []string{": desc", "b"},
			wantErr: "option keys must not be blank",
		},
		{
			name:    "duplicate key",
			raw:     []string{"a: 1", "a: 2"},
			wantErr: "duplicate option key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			criteria, err := parseChoiceOptions(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("parseChoiceOptions() = %v, want error containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseChoiceOptions() error = %v", err)
			}
			if got, want := criteria.Keys(), tc.wantKeys; !equalStrings(got, want) {
				t.Errorf("keys = %v, want %v", got, want)
			}
			for key, want := range tc.wantDesc {
				if got := criteria.Get(key); got != want {
					t.Errorf("description[%q] = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestRunChoice(t *testing.T) {
	t.Cleanup(func() {
		resetChoiceFlags()
		resetSubjectFlags()
	})

	baseSetup := func(host string) {
		resetChoiceFlags()
		resetSubjectFlags()
		subjectFlags.model = "clef"
		subjectFlags.host = host
		subjectFlags.timeout = 5 * time.Second
		choiceFlags.question = "Which team should handle this ticket?"
	}

	tests := []struct {
		name            string
		setup           func(host string)
		respBody        string
		wantOut         string
		wantErrContains string
		wantRaw         bool
		checkRaw        func(t *testing.T, raw []byte)
	}{
		{
			name: "chooses winner",
			setup: func(host string) {
				baseSetup(host)
				subjectFlags.prompt = "I was charged twice."
				choiceFlags.options = []string{"billing: Payments and refunds", "technical: Bugs", "other: None of the above"}
			},
			respBody: `{"model":"clef","answers":{"choice":{"type":"choice","choice":"billing","probabilities":{"billing":0.9,"technical":0.07,"other":0.03},"confidence":0.85}},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantOut:  "billing\n",
			wantRaw:  true,
			checkRaw: func(t *testing.T, raw []byte) {
				// Option order must follow flag order for tie-breaking.
				billing := strings.Index(string(raw), `"billing"`)
				technical := strings.Index(string(raw), `"technical"`)
				other := strings.Index(string(raw), `"other"`)
				if billing < 0 || technical < 0 || other < 0 || billing > technical || technical > other {
					t.Errorf("option order not preserved in %s", raw)
				}

				var req struct {
					Model     string `json:"model"`
					State     string `json:"state"`
					Questions map[string]struct {
						Type         string            `json:"type"`
						Instructions string            `json:"instructions"`
						Criteria     map[string]string `json:"criteria"`
					} `json:"questions"`
				}
				if err := json.Unmarshal(raw, &req); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if req.Model != "clef" {
					t.Errorf("model = %q", req.Model)
				}
				if req.State != "I was charged twice." {
					t.Errorf("state = %q", req.State)
				}
				q, ok := req.Questions[choiceKey]
				if !ok || q.Type != "choice" || q.Instructions != "Which team should handle this ticket?" {
					t.Errorf("questions = %+v", req.Questions)
				}
				if !ok || len(q.Criteria) != 3 {
					t.Errorf("criteria = %+v, want 3 options", q.Criteria)
				}
			},
		},
		{
			name: "one option rejected",
			setup: func(host string) {
				baseSetup(host)
				subjectFlags.prompt = "I was charged twice."
				choiceFlags.options = []string{"billing"}
			},
			wantErrContains: "2 to 26 options are required",
		},
		{
			name: "missing answer",
			setup: func(host string) {
				baseSetup(host)
				subjectFlags.prompt = "I was charged twice."
				choiceFlags.options = []string{"billing: Payments", "technical: Bugs"}
			},
			respBody:        `{"model":"clef","answers":{},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantErrContains: "no answer returned",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				raw, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				_, _ = w.Write([]byte(tc.respBody))
			}))
			t.Cleanup(srv.Close)

			tc.setup(srv.URL)
			out, err := captureStdout(t, func() error { return runChoice(choiceCmd, nil) })

			if tc.wantOut != "" && out != tc.wantOut {
				t.Errorf("stdout = %q, want %q", out, tc.wantOut)
			}
			if tc.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("err = %v, want error containing %q", err, tc.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if tc.wantRaw && len(raw) == 0 {
				t.Error("expected a request to be sent")
			}
			if tc.checkRaw != nil {
				tc.checkRaw(t, raw)
			}
		})
	}
}
