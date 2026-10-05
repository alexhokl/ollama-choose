package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexhokl/ollama-choose/internal/ollama"
)

// choiceKey names the single choice question sent to System One.
const choiceKey = "choice"

type choiceOptions struct {
	question string
	options  []string
}

var choiceFlags choiceOptions

var choiceCmd = &cobra.Command{
	Use:   "choice",
	Short: "Choose one of several options for an image, text file, or prompt",
	Example: `  ollama-choose choice --model clef --prompt "I was charged twice. Please refund the extra payment." \
  --question "Which team should handle this ticket?" \
  --option "billing: Payments and refunds" --option "technical: Bugs and integrations" --option "other: None of the above"`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runChoice,
}

func init() {
	addSubjectFlags(choiceCmd)
	choiceCmd.Flags().StringVar(&choiceFlags.question, "question", "", "question to answer with one of the options (required)")
	choiceCmd.Flags().StringArrayVar(&choiceFlags.options, "option", nil, "option as key or key: description; repeat for each option (2 to 26)")
	_ = choiceCmd.MarkFlagRequired("question")

	rootCmd.AddCommand(choiceCmd)
}

func runChoice(cmd *cobra.Command, _ []string) error {
	if err := validateChoiceFlags(); err != nil {
		return err
	}
	criteria, err := parseChoiceOptions(choiceFlags.options)
	if err != nil {
		return err
	}

	resp, err := runSystemOne(cmd, map[string]ollama.SystemOneQuestion{
		choiceKey: {Type: "choice", Instructions: choiceFlags.question, Criteria: criteria},
	})
	if err != nil {
		return err
	}

	answer, err := answerFor(resp, choiceKey, "choice")
	if err != nil {
		return err
	}

	if subjectFlags.verbose {
		fmt.Fprintf(os.Stderr, "confidence=%.4f\n", answer.Confidence)
		for _, key := range criteria.Keys() {
			fmt.Fprintf(os.Stderr, "probability %s %.4f\n", key, answer.Probabilities[key])
		}
	}

	fmt.Println(answer.Choice)
	return nil
}

// validateChoiceFlags enforces a nonempty question.
func validateChoiceFlags() error {
	if strings.TrimSpace(choiceFlags.question) == "" {
		return fmt.Errorf("--question must not be empty")
	}
	return nil
}

// parseChoiceOptions converts repeated --option values (key or key: description)
// into ordered criteria. Keys must be nonblank and unique; 2 to 26 options
// are required. A missing description uses the key itself.
func parseChoiceOptions(raw []string) (*ollama.CriteriaMap, error) {
	criteria := ollama.NewCriteriaMap()
	for _, opt := range raw {
		key, description, _ := strings.Cut(opt, ":")
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("option keys must not be blank: %q", opt)
		}
		if criteria.Has(key) {
			return nil, fmt.Errorf("duplicate option key %q", key)
		}
		if description = strings.TrimSpace(description); description == "" {
			description = key
		}
		criteria.Set(key, description)
	}
	if n := criteria.Len(); n < 2 || n > 26 {
		return nil, fmt.Errorf("2 to 26 options are required, got %d", n)
	}
	return criteria, nil
}
