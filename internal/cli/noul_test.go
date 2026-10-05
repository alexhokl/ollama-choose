package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexhokl/ollama-choose/internal/ollama"
)

func resetNoulFlags() {
	noulFlags = noulOptions{}
	noulCmd.SilenceErrors = false
}

func TestValidateNoulFlags(t *testing.T) {
	t.Cleanup(resetNoulFlags)

	tests := []struct {
		name    string
		setup   func()
		wantErr string
	}{
		{
			name:    "no subject",
			setup:   func() {},
			wantErr: "one of --image, --file, or --prompt is required",
		},
		{
			name:  "prompt only",
			setup: func() { noulFlags.prompt = "hello" },
		},
		{
			name:  "image only",
			setup: func() { noulFlags.image = "a.jpg" },
		},
		{
			name:  "file only",
			setup: func() { noulFlags.file = "a.txt" },
		},
		{
			name: "image and file",
			setup: func() {
				noulFlags.image = "a.jpg"
				noulFlags.file = "a.txt"
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "all three",
			setup: func() {
				noulFlags.image = "a.jpg"
				noulFlags.file = "a.txt"
				noulFlags.prompt = "hello"
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "threshold below range",
			setup: func() {
				noulFlags.prompt = "hello"
				noulFlags.threshold = -0.1
			},
			wantErr: "--threshold must be between 0 and 1",
		},
		{
			name: "threshold above range",
			setup: func() {
				noulFlags.prompt = "hello"
				noulFlags.threshold = 1.1
			},
			wantErr: "--threshold must be between 0 and 1",
		},
		{
			name: "threshold zero valid",
			setup: func() {
				noulFlags.prompt = "hello"
				noulFlags.threshold = 0
			},
		},
		{
			name: "threshold one valid",
			setup: func() {
				noulFlags.prompt = "hello"
				noulFlags.threshold = 1
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetNoulFlags()
			noulFlags.model = "m"
			noulFlags.statement = "s"
			tc.setup()
			err := validateNoulFlags()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("validateNoulFlags() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("validateNoulFlags() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolveHost(t *testing.T) {
	t.Cleanup(resetNoulFlags)

	t.Setenv("OLLAMA_HOST", "http://envhost:1234")
	noulFlags.host = ""
	if got := resolveHost(); got != "http://envhost:1234" {
		t.Errorf("env host: resolveHost() = %q", got)
	}

	noulFlags.host = "http://flaghost:5678"
	if got := resolveHost(); got != "http://flaghost:5678" {
		t.Errorf("flag host: resolveHost() = %q", got)
	}

	t.Setenv("OLLAMA_HOST", "")
	noulFlags.host = "127.0.0.1:11434"
	if got := resolveHost(); got != "http://127.0.0.1:11434" {
		t.Errorf("scheme-less host: resolveHost() = %q", got)
	}

	noulFlags.host = ""
	if got := resolveHost(); got != defaultHost {
		t.Errorf("default host: resolveHost() = %q, want %q", got, defaultHost)
	}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	fnErr := fn()
	os.Stdout = orig
	_ = w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(data), fnErr
}

func TestRunNoul(t *testing.T) {
	t.Cleanup(resetNoulFlags)

	imagePath := filepath.Join(t.TempDir(), "img.png")
	imageBytes := []byte{0x89, 0x50, 0x4e, 0x47}
	if err := os.WriteFile(imagePath, imageBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(filePath, []byte("Bonjour tout le monde"), 0o600); err != nil {
		t.Fatal(err)
	}

	baseSetup := func(host string) {
		noulFlags.model = "clef-flash:9b"
		noulFlags.statement = "the input is written in English"
		noulFlags.host = host
		noulFlags.timeout = 5 * time.Second
		noulFlags.threshold = 0.5
	}

	tests := []struct {
		name       string
		setup      func(host string)
		respBody   string
		wantOut    string
		wantErrIs  error
		wantErrAny bool
		checkReq   func(t *testing.T, req *ollama.SystemOneRequest)
	}{
		{
			name: "prompt judged true",
			setup: func(host string) {
				baseSetup(host)
				noulFlags.prompt = "Hello world"
			},
			respBody: `{"model":"clef-flash:9b","answers":{"statement":{"type":"noul","noul":0.9}},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantOut:  "true\n",
			checkReq: func(t *testing.T, req *ollama.SystemOneRequest) {
				if req.Model != "clef-flash:9b" {
					t.Errorf("model = %q", req.Model)
				}
				if req.State != "Hello world" {
					t.Errorf("state = %q", req.State)
				}
				q, ok := req.Questions[statementKey]
				if !ok || q.Type != "noul" || q.Instructions != "the input is written in English" {
					t.Errorf("questions = %+v", req.Questions)
				}
				if len(req.Images) != 0 {
					t.Errorf("images = %v, want none", req.Images)
				}
			},
		},
		{
			name: "text file judged true",
			setup: func(host string) {
				baseSetup(host)
				noulFlags.file = filePath
			},
			respBody: `{"model":"clef-flash:9b","answers":{"statement":{"type":"noul","noul":0.95}},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantOut:  "true\n",
			checkReq: func(t *testing.T, req *ollama.SystemOneRequest) {
				if req.State != "Bonjour tout le monde" {
					t.Errorf("state = %q, want file contents", req.State)
				}
			},
		},
		{
			name: "image judged false",
			setup: func(host string) {
				baseSetup(host)
				noulFlags.image = imagePath
			},
			respBody:  `{"model":"clef-flash:9b","answers":{"statement":{"type":"noul","noul":0.005}},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantOut:   "false\n",
			wantErrIs: ErrJudgedFalse,
			checkReq: func(t *testing.T, req *ollama.SystemOneRequest) {
				want := base64.StdEncoding.EncodeToString(imageBytes)
				if len(req.Images) != 1 || req.Images[0] != want {
					t.Errorf("images = %v, want [%s]", req.Images, want)
				}
				if req.State != imageState {
					t.Errorf("state = %q, want %q", req.State, imageState)
				}
			},
		},
		{
			name: "threshold above score",
			setup: func(host string) {
				baseSetup(host)
				noulFlags.prompt = "Hello world"
				noulFlags.threshold = 0.95
			},
			respBody:  `{"model":"clef-flash:9b","answers":{"statement":{"type":"noul","noul":0.9}},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantOut:   "false\n",
			wantErrIs: ErrJudgedFalse,
		},
		{
			name: "missing answer",
			setup: func(host string) {
				baseSetup(host)
				noulFlags.prompt = "Hello world"
			},
			respBody:   `{"model":"clef-flash:9b","answers":{},"usage":{"input_tokens":1,"output_tokens":0}}`,
			wantErrAny: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetNoulFlags()
			var captured ollama.SystemOneRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Errorf("decode request: %v", err)
				}
				_, _ = w.Write([]byte(tc.respBody))
			}))
			t.Cleanup(srv.Close)

			tc.setup(srv.URL)
			out, err := captureStdout(t, func() error { return runNoul(noulCmd, nil) })

			if tc.wantOut != "" && out != tc.wantOut {
				t.Errorf("stdout = %q, want %q", out, tc.wantOut)
			}
			switch {
			case tc.wantErrIs != nil:
				if !errors.Is(err, tc.wantErrIs) {
					t.Errorf("err = %v, want %v", err, tc.wantErrIs)
				}
			case tc.wantErrAny:
				if err == nil || errors.Is(err, ErrJudgedFalse) {
					t.Errorf("err = %v, want generic error", err)
				}
			default:
				if err != nil {
					t.Errorf("err = %v, want nil", err)
				}
			}
			if tc.checkReq != nil {
				tc.checkReq(t, &captured)
			}
		})
	}
}
