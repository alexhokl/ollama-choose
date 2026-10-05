package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexhokl/ollama-choose/internal/ollama"
)

const defaultHost = "http://localhost:11434"

// imageState is the state sent when judging an image without accompanying
// text; System One always requires a nonempty state.
const imageState = "An image is attached to this decision."

// subjectOptions holds the flags shared by every decision command. The same
// variables are bound by each command's flag set; only the command being
// executed parses flags into them.
type subjectOptions struct {
	model   string
	image   string
	file    string
	prompt  string
	host    string
	timeout time.Duration
	verbose bool
}

var subjectFlags subjectOptions

// addSubjectFlags registers the shared decision-command flags on cmd.
func addSubjectFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&subjectFlags.model, "model", "", "Ollama decision model to judge with, e.g. clef or clef-flash (required)")
	flags.StringVar(&subjectFlags.image, "image", "", "path to an image file to judge")
	flags.StringVar(&subjectFlags.file, "file", "", "path to a text file to judge")
	flags.StringVar(&subjectFlags.prompt, "prompt", "", "inline text to judge")
	flags.StringVar(&subjectFlags.host, "host", "", "Ollama host URL (defaults to $OLLAMA_HOST or "+defaultHost+")")
	flags.DurationVar(&subjectFlags.timeout, "timeout", 2*time.Minute, "request timeout")
	flags.BoolVar(&subjectFlags.verbose, "verbose", false, "print additional answer details to stderr")

	_ = cmd.MarkFlagRequired("model")
}

// validateSubjectFlags enforces a nonempty model and exactly one subject
// input.
func validateSubjectFlags() error {
	if strings.TrimSpace(subjectFlags.model) == "" {
		return fmt.Errorf("--model must not be empty")
	}
	subjects := map[string]string{
		"--image":  subjectFlags.image,
		"--file":   subjectFlags.file,
		"--prompt": subjectFlags.prompt,
	}
	set := 0
	for _, v := range subjects {
		if v != "" {
			set++
		}
	}
	switch {
	case set == 0:
		return fmt.Errorf("one of --image, --file, or --prompt is required")
	case set > 1:
		return fmt.Errorf("--image, --file, and --prompt are mutually exclusive; provide exactly one")
	}
	return nil
}

// loadSubject reads the selected subject, returning the System One state and
// base64-encoded images for image inputs.
func loadSubject() (string, []string, error) {
	switch {
	case subjectFlags.image != "":
		data, err := os.ReadFile(subjectFlags.image)
		if err != nil {
			return "", nil, fmt.Errorf("read image: %w", err)
		}
		return imageState, []string{base64.StdEncoding.EncodeToString(data)}, nil
	case subjectFlags.file != "":
		data, err := os.ReadFile(subjectFlags.file)
		if err != nil {
			return "", nil, fmt.Errorf("read file: %w", err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", nil, fmt.Errorf("the input file is empty")
		}
		return string(data), nil, nil
	default:
		if strings.TrimSpace(subjectFlags.prompt) == "" {
			return "", nil, fmt.Errorf("--prompt must not be empty")
		}
		return subjectFlags.prompt, nil, nil
	}
}

// resolveHost picks the Ollama host URL: --host wins, then OLLAMA_HOST,
// then the localhost default. Scheme-less values get an http:// prefix.
func resolveHost() string {
	host := subjectFlags.host
	if host == "" {
		host = os.Getenv("OLLAMA_HOST")
	}
	if host == "" {
		host = defaultHost
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	return strings.TrimRight(host, "/")
}

// runSystemOne validates the shared flags, loads the subject, and sends the
// given questions to System One.
func runSystemOne(cmd *cobra.Command, questions map[string]ollama.SystemOneQuestion) (*ollama.SystemOneResponse, error) {
	if err := validateSubjectFlags(); err != nil {
		return nil, err
	}
	state, images, err := loadSubject()
	if err != nil {
		return nil, err
	}
	client := ollama.NewClient(resolveHost(), subjectFlags.timeout)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := client.SystemOne(ctx, ollama.SystemOneRequest{
		Model:     subjectFlags.model,
		State:     state,
		Images:    images,
		Questions: questions,
	})
	if err != nil {
		return nil, fmt.Errorf("judge: %w", err)
	}
	return resp, nil
}

// answerFor returns the answer for a question, checking that it exists and
// has the expected type.
func answerFor(resp *ollama.SystemOneResponse, key, wantType string) (ollama.SystemOneAnswer, error) {
	answer, ok := resp.Answers[key]
	if !ok {
		return ollama.SystemOneAnswer{}, fmt.Errorf("judge: no answer returned for the question")
	}
	if answer.Type != wantType {
		return ollama.SystemOneAnswer{}, fmt.Errorf("judge: unexpected answer type %q", answer.Type)
	}
	return answer, nil
}
