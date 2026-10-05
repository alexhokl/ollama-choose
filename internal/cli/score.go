package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexhokl/ollama-choose/internal/ollama"
)

// scoreKey names the single score question sent to System One.
const scoreKey = "score"

type scoreOptions struct {
	question string
	levels   []string
}

var scoreFlags scoreOptions

var scoreCmd = &cobra.Command{
	Use:   "score",
	Short: "Score an image, text file, or prompt on an ordered scale",
	Example: `  ollama-choose score --model clef --prompt "Our checkout has returned 500 errors since 9am." \
  --question "How urgent is this ticket?" \
  --level "Routine: no time pressure" --level "Soon: a customer is inconvenienced" --level "Immediate: a critical service is unavailable"`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runScore,
}

func init() {
	addSubjectFlags(scoreCmd)
	scoreCmd.Flags().StringVar(&scoreFlags.question, "question", "", "question to answer with a score on the scale (required)")
	scoreCmd.Flags().StringArrayVar(&scoreFlags.levels, "level", nil, "scale level, lowest first; repeat for each level (2 to 26)")
	_ = scoreCmd.MarkFlagRequired("question")

	rootCmd.AddCommand(scoreCmd)
}

func runScore(cmd *cobra.Command, _ []string) error {
	if err := validateScoreFlags(); err != nil {
		return err
	}
	levels, err := parseScoreLevels(scoreFlags.levels)
	if err != nil {
		return err
	}

	resp, err := runSystemOne(cmd, map[string]ollama.SystemOneQuestion{
		scoreKey: {Type: "score", Instructions: scoreFlags.question, Criteria: levels},
	})
	if err != nil {
		return err
	}

	answer, err := answerFor(resp, scoreKey, "score")
	if err != nil {
		return err
	}

	if subjectFlags.verbose {
		fmt.Fprintf(os.Stderr, "confidence=%.4f\n", answer.Confidence)
		for i, level := range levels {
			fmt.Fprintf(os.Stderr, "probability %d %.4f %s\n", i, answer.Probabilities[strconv.Itoa(i)], level)
		}
	}

	fmt.Println(answer.Score)
	return nil
}

// validateScoreFlags enforces a nonempty question.
func validateScoreFlags() error {
	if strings.TrimSpace(scoreFlags.question) == "" {
		return fmt.Errorf("--question must not be empty")
	}
	return nil
}

// parseScoreLevels converts repeated --level values into the ordered scale
// (lowest first). Levels must be nonblank; 2 to 26 are required.
func parseScoreLevels(raw []string) ([]string, error) {
	levels := make([]string, 0, len(raw))
	for _, level := range raw {
		if level = strings.TrimSpace(level); level == "" {
			return nil, fmt.Errorf("scale levels must not be blank")
		}
		levels = append(levels, level)
	}
	if n := len(levels); n < 2 || n > 26 {
		return nil, fmt.Errorf("2 to 26 levels are required, got %d", n)
	}
	return levels, nil
}
