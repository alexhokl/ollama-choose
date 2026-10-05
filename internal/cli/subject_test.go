package cli

import (
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetSubjectFlags() {
	subjectFlags = subjectOptions{}
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

func TestValidateSubjectFlags(t *testing.T) {
	t.Cleanup(resetSubjectFlags)

	tests := []struct {
		name    string
		setup   func()
		wantErr string
	}{
		{
			name:    "no model",
			setup:   func() { subjectFlags.prompt = "hello" },
			wantErr: "--model must not be empty",
		},
		{
			name:    "no subject",
			setup:   func() { subjectFlags.model = "m" },
			wantErr: "one of --image, --file, or --prompt is required",
		},
		{
			name:  "prompt only",
			setup: func() { subjectFlags.model = "m"; subjectFlags.prompt = "hello" },
		},
		{
			name:  "image only",
			setup: func() { subjectFlags.model = "m"; subjectFlags.image = "a.jpg" },
		},
		{
			name:  "file only",
			setup: func() { subjectFlags.model = "m"; subjectFlags.file = "a.txt" },
		},
		{
			name: "image and file",
			setup: func() {
				subjectFlags.model = "m"
				subjectFlags.image = "a.jpg"
				subjectFlags.file = "a.txt"
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "all three",
			setup: func() {
				subjectFlags.model = "m"
				subjectFlags.image = "a.jpg"
				subjectFlags.file = "a.txt"
				subjectFlags.prompt = "hello"
			},
			wantErr: "mutually exclusive",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetSubjectFlags()
			tc.setup()
			err := validateSubjectFlags()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("validateSubjectFlags() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("validateSubjectFlags() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadSubject(t *testing.T) {
	t.Cleanup(resetSubjectFlags)

	imagePath := filepath.Join(t.TempDir(), "img.png")
	imageBytes := []byte{0x89, 0x50, 0x4e, 0x47}
	if err := os.WriteFile(imagePath, imageBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(filePath, []byte("Bonjour"), 0o600); err != nil {
		t.Fatal(err)
	}
	blankPath := filepath.Join(t.TempDir(), "blank.txt")
	if err := os.WriteFile(blankPath, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("prompt", func(t *testing.T) {
		resetSubjectFlags()
		subjectFlags.prompt = "Hello world"
		state, images, err := loadSubject()
		if err != nil || state != "Hello world" || len(images) != 0 {
			t.Errorf("loadSubject() = %q, %v, %v", state, images, err)
		}
	})

	t.Run("blank prompt", func(t *testing.T) {
		resetSubjectFlags()
		subjectFlags.prompt = "   "
		if _, _, err := loadSubject(); err == nil || !strings.Contains(err.Error(), "--prompt must not be empty") {
			t.Errorf("loadSubject() = %v, want empty prompt error", err)
		}
	})

	t.Run("file", func(t *testing.T) {
		resetSubjectFlags()
		subjectFlags.file = filePath
		state, images, err := loadSubject()
		if err != nil || state != "Bonjour" || len(images) != 0 {
			t.Errorf("loadSubject() = %q, %v, %v", state, images, err)
		}
	})

	t.Run("blank file", func(t *testing.T) {
		resetSubjectFlags()
		subjectFlags.file = blankPath
		if _, _, err := loadSubject(); err == nil || !strings.Contains(err.Error(), "the input file is empty") {
			t.Errorf("loadSubject() = %v, want empty file error", err)
		}
	})

	t.Run("image", func(t *testing.T) {
		resetSubjectFlags()
		subjectFlags.image = imagePath
		state, images, err := loadSubject()
		want := base64.StdEncoding.EncodeToString(imageBytes)
		if err != nil || state != imageState || len(images) != 1 || images[0] != want {
			t.Errorf("loadSubject() = %q, %v, %v", state, images, err)
		}
	})

	t.Run("missing image", func(t *testing.T) {
		resetSubjectFlags()
		subjectFlags.image = "/nonexistent/image.png"
		if _, _, err := loadSubject(); err == nil || !strings.Contains(err.Error(), "read image") {
			t.Errorf("loadSubject() = %v, want read error", err)
		}
	})
}

func TestResolveHost(t *testing.T) {
	t.Cleanup(resetSubjectFlags)

	t.Setenv("OLLAMA_HOST", "http://envhost:1234")
	subjectFlags.host = ""
	if got := resolveHost(); got != "http://envhost:1234" {
		t.Errorf("env host: resolveHost() = %q", got)
	}

	subjectFlags.host = "http://flaghost:5678"
	if got := resolveHost(); got != "http://flaghost:5678" {
		t.Errorf("flag host: resolveHost() = %q", got)
	}

	t.Setenv("OLLAMA_HOST", "")
	subjectFlags.host = "127.0.0.1:11434"
	if got := resolveHost(); got != "http://127.0.0.1:11434" {
		t.Errorf("scheme-less host: resolveHost() = %q", got)
	}

	subjectFlags.host = ""
	if got := resolveHost(); got != defaultHost {
		t.Errorf("default host: resolveHost() = %q, want %q", got, defaultHost)
	}
}
