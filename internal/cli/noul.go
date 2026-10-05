package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexhokl/ollama-choose/internal/ollama"
)

// ErrJudgedFalse is returned by the noul command when the model judges the
// statement false. Execute maps it to exit code 1 without printing it as an
// error, since a false judgment is a valid outcome.
var ErrJudgedFalse = errors.New("judged false")

const defaultHost = "http://localhost:11434"

// imageState is the state sent when judging an image without accompanying
// text; System One always requires a nonempty state.
const imageState = "An image is attached to this decision."

// statementKey names the single noul question sent to System One.
const statementKey = "statement"

type noulOptions struct {
	model     string
	statement string
	image     string
	file      string
	prompt    string
	host      string
	timeout   time.Duration
	threshold float64
	verbose   bool
}

var noulFlags noulOptions

var noulCmd = &cobra.Command{
	Use:   "noul",
	Short: "Judge a boolean statement against an image, text file, or prompt",
	Example: `  ollama-choose noul --model clef --image photo.jpg --statement "the image shows a dog"
  ollama-choose noul --model clef-flash --file notes.txt --statement "the text is written in English"
  ollama-choose noul --model clef-flash --prompt "Paris is the capital of France" --statement "this is factually correct"`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runNoul,
}

func init() {
	flags := noulCmd.Flags()
	flags.StringVar(&noulFlags.model, "model", "", "Ollama decision model to judge with, e.g. clef or clef-flash (required)")
	flags.StringVar(&noulFlags.statement, "statement", "", "boolean statement to judge against the input (required)")
	flags.StringVar(&noulFlags.image, "image", "", "path to an image file to judge")
	flags.StringVar(&noulFlags.file, "file", "", "path to a text file to judge")
	flags.StringVar(&noulFlags.prompt, "prompt", "", "inline text to judge")
	flags.StringVar(&noulFlags.host, "host", "", "Ollama host URL (defaults to $OLLAMA_HOST or "+defaultHost+")")
	flags.DurationVar(&noulFlags.timeout, "timeout", 2*time.Minute, "request timeout")
	flags.Float64Var(&noulFlags.threshold, "threshold", 0.5, "score at or above which the statement is judged true (0 to 1)")
	flags.BoolVar(&noulFlags.verbose, "verbose", false, "print the raw score to stderr")

	_ = noulCmd.MarkFlagRequired("model")
	_ = noulCmd.MarkFlagRequired("statement")

	rootCmd.AddCommand(noulCmd)
}

func runNoul(cmd *cobra.Command, _ []string) error {
	if err := validateNoulFlags(); err != nil {
		return err
	}

	state, images, err := loadSubject()
	if err != nil {
		return err
	}

	client := ollama.NewClient(resolveHost(), noulFlags.timeout)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := client.SystemOne(ctx, ollama.SystemOneRequest{
		Model:  noulFlags.model,
		State:  state,
		Images: images,
		Questions: map[string]ollama.SystemOneQuestion{
			statementKey: {Type: "noul", Instructions: noulFlags.statement},
		},
	})
	if err != nil {
		return fmt.Errorf("judge: %w", err)
	}

	answer, ok := resp.Answers[statementKey]
	if !ok {
		return fmt.Errorf("judge: no answer returned for the statement")
	}
	if answer.Type != "noul" {
		return fmt.Errorf("judge: unexpected answer type %q", answer.Type)
	}

	if noulFlags.verbose {
		fmt.Fprintf(os.Stderr, "score=%.4f\n", answer.Noul)
	}

	if answer.Noul >= noulFlags.threshold {
		fmt.Println("true")
		return nil
	}
	fmt.Println("false")
	cmd.SilenceErrors = true
	return ErrJudgedFalse
}

// validateNoulFlags enforces non-empty flags, exactly one subject input, and
// a threshold within [0, 1].
func validateNoulFlags() error {
	if strings.TrimSpace(noulFlags.model) == "" {
		return fmt.Errorf("--model must not be empty")
	}
	if strings.TrimSpace(noulFlags.statement) == "" {
		return fmt.Errorf("--statement must not be empty")
	}
	if noulFlags.threshold < 0 || noulFlags.threshold > 1 {
		return fmt.Errorf("--threshold must be between 0 and 1")
	}

	subjects := map[string]string{
		"--image":  noulFlags.image,
		"--file":   noulFlags.file,
		"--prompt": noulFlags.prompt,
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
	case noulFlags.image != "":
		data, err := os.ReadFile(noulFlags.image)
		if err != nil {
			return "", nil, fmt.Errorf("read image: %w", err)
		}
		return imageState, []string{base64.StdEncoding.EncodeToString(data)}, nil
	case noulFlags.file != "":
		data, err := os.ReadFile(noulFlags.file)
		if err != nil {
			return "", nil, fmt.Errorf("read file: %w", err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", nil, fmt.Errorf("the input file is empty")
		}
		return string(data), nil, nil
	default:
		if strings.TrimSpace(noulFlags.prompt) == "" {
			return "", nil, fmt.Errorf("--prompt must not be empty")
		}
		return noulFlags.prompt, nil, nil
	}
}

// resolveHost picks the Ollama host URL: --host wins, then OLLAMA_HOST,
// then the localhost default. Scheme-less values get an http:// prefix.
func resolveHost() string {
	host := noulFlags.host
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
