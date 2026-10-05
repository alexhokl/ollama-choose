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

func resetScoreFlags() {
	scoreFlags = scoreOptions{}
}

func TestParseScoreLevels(t *testing.T) {
	tooMany := make([]string, 27)
	for i := range tooMany {
		tooMany[i] = "level" + strconv.Itoa(i)
	}

	tests := []struct {
		name    string
		raw     []string
		want    []string
		wantErr string
	}{
		{
			name: "three levels",
			raw:  []string{"Routine: no time pressure", "Soon", "Urgent"},
			want: []string{"Routine: no time pressure", "Soon", "Urgent"},
		},
		{
			name:    "one level",
			raw:     []string{"Routine"},
			wantErr: "2 to 26 levels are required",
		},
		{
			name:    "too many levels",
			raw:     tooMany,
			wantErr: "2 to 26 levels are required",
		},
		{
			name:    "blank level",
			raw:     []string{"Routine", "  "},
			wantErr: "scale levels must not be blank",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			levels, err := parseScoreLevels(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("parseScoreLevels() = %v, want error containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseScoreLevels() error = %v", err)
			}
			if len(levels) != len(tc.want) {
				t.Fatalf("levels = %v, want %v", levels, tc.want)
			}
			for i := range levels {
				if levels[i] != tc.want[i] {
					t.Errorf("levels[%d] = %q, want %q", i, levels[i], tc.want[i])
				}
			}
		})
	}
}

func TestRunScore(t *testing.T) {
	t.Cleanup(func() {
		resetScoreFlags()
		resetSubjectFlags()
	})

	levels := []string{"Routine: no time pressure", "Soon: a customer is inconvenienced", "Immediate: a critical service is unavailable"}

	baseSetup := func(host string) {
		resetScoreFlags()
		resetSubjectFlags()
		subjectFlags.model = "clef"
		subjectFlags.host = host
		subjectFlags.timeout = 5 * time.Second
		scoreFlags.question = "How urgent is this ticket?"
	}

	tests := []struct {
		name            string
		setup           func(host string)
		respBody        string
		wantOut         string
		wantErrContains string
		checkReq        func(t *testing.T, raw []byte)
	}{
		{
			name: "prints weighted score",
			setup: func(host string) {
				baseSetup(host)
				subjectFlags.prompt = "Our checkout has returned 500 errors since 9am."
				scoreFlags.levels = levels
			},
			respBody: `{"model":"clef","answers":{"score":{"type":"score","score":0.704,"legend":{"0":"Routine: no time pressure","1":"Soon: a customer is inconvenienced","2":"Immediate: a critical service is unavailable"},"probabilities":{"0":0.451,"1":0.353,"2":0.196},"confidence":0.071}},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantOut:  "0.704\n",
			checkReq: func(t *testing.T, raw []byte) {
				var req struct {
					Model     string `json:"model"`
					State     string `json:"state"`
					Questions map[string]struct {
						Type         string   `json:"type"`
						Instructions string   `json:"instructions"`
						Criteria     []string `json:"criteria"`
					} `json:"questions"`
				}
				if err := json.Unmarshal(raw, &req); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if req.Model != "clef" {
					t.Errorf("model = %q", req.Model)
				}
				if req.State != "Our checkout has returned 500 errors since 9am." {
					t.Errorf("state = %q", req.State)
				}
				q, ok := req.Questions[scoreKey]
				if !ok || q.Type != "score" || q.Instructions != "How urgent is this ticket?" {
					t.Errorf("questions = %+v", req.Questions)
				}
				if !ok || len(q.Criteria) != len(levels) {
					t.Fatalf("criteria = %+v, want %d levels", q.Criteria, len(levels))
				}
				for i, want := range levels {
					if q.Criteria[i] != want {
						t.Errorf("criteria[%d] = %q, want %q", i, q.Criteria[i], want)
					}
				}
			},
		},
		{
			name: "one level rejected",
			setup: func(host string) {
				baseSetup(host)
				subjectFlags.prompt = "Our checkout has returned 500 errors since 9am."
				scoreFlags.levels = []string{"Routine"}
			},
			wantErrContains: "2 to 26 levels are required",
		},
		{
			name: "missing answer",
			setup: func(host string) {
				baseSetup(host)
				subjectFlags.prompt = "Our checkout has returned 500 errors since 9am."
				scoreFlags.levels = levels
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
			out, err := captureStdout(t, func() error { return runScore(scoreCmd, nil) })

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
			if tc.checkReq != nil {
				tc.checkReq(t, raw)
			}
		})
	}
}
