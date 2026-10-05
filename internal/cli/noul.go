package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexhokl/ollama-choose/internal/ollama"
)

// ErrJudgedFalse is returned by the noul command when the model judges the
// statement false. Execute maps it to exit code 1 without printing it as an
// error, since a false judgment is a valid outcome.
var ErrJudgedFalse = errors.New("judged false")

// statementKey names the single noul question sent to System One.
const statementKey = "statement"

type noulOptions struct {
	question  string
	threshold float64
}

var noulFlags noulOptions

var noulCmd = &cobra.Command{
	Use:   "noul",
	Short: "Judge a boolean statement against an image, text file, or prompt",
	Example: `  ollama-choose noul --model clef --image photo.jpg --question "the image shows a dog"
  ollama-choose noul --model clef-flash --file notes.txt --question "the text is written in English"
  ollama-choose noul --model clef-flash --prompt "Paris is the capital of France" --question "this is factually correct"`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runNoul,
}

func init() {
	addSubjectFlags(noulCmd)
	noulCmd.Flags().StringVar(&noulFlags.question, "question", "", "boolean statement to judge against the input (required)")
	noulCmd.Flags().Float64Var(&noulFlags.threshold, "threshold", 0.5, "score at or above which the statement is judged true (0 to 1)")
	_ = noulCmd.MarkFlagRequired("question")

	rootCmd.AddCommand(noulCmd)
}

func runNoul(cmd *cobra.Command, _ []string) error {
	if err := validateNoulFlags(); err != nil {
		return err
	}

	resp, err := runSystemOne(cmd, map[string]ollama.SystemOneQuestion{
		statementKey: {Type: "noul", Instructions: noulFlags.question},
	})
	if err != nil {
		return err
	}

	answer, err := answerFor(resp, statementKey, "noul")
	if err != nil {
		return err
	}

	if subjectFlags.verbose {
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

// validateNoulFlags enforces a nonempty question and a threshold within
// [0, 1].
func validateNoulFlags() error {
	if strings.TrimSpace(noulFlags.question) == "" {
		return fmt.Errorf("--question must not be empty")
	}
	if noulFlags.threshold < 0 || noulFlags.threshold > 1 {
		return fmt.Errorf("--threshold must be between 0 and 1")
	}
	return nil
}
