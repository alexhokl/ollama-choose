# ollama-choose

A CLI for [Ollama](https://ollama.com)'s System One decision API (`/v1/systemone`). It judges one subject — an image, a text file, or inline text — using local decision models such as [clef](https://ollama.com/library/clef) and [clef-flash](https://ollama.com/library/clef-flash).

## Requirements

- [Ollama](https://ollama.com) v0.35.1 or newer (System One was added in v0.35.1)
- A decision model available locally:

  ```sh
  ollama pull clef-flash:9b
  ```

- Go 1.27+ if building from source

## Installation

```sh
go install github.com/alexhokl/ollama-choose/cmd/ollama-choose@latest
```

Or from a checkout of this repository:

```sh
task install    # go install ./cmd/ollama-choose, into GOBIN
task binary     # build ./bin/ollama-choose
```

## Usage

```
ollama-choose <command> --model MODEL (--image PATH | --file PATH | --prompt TEXT) [flags]
```

Every command sends one subject and one question to the model, and prints the result on stdout. `--verbose` details print on stderr.

| Flag | Description |
| --- | --- |
| `--model` | Decision model to judge with, e.g. `clef-flash:9b` (required) |
| `--image` | Path to an image file to judge |
| `--file` | Path to a text file to judge |
| `--prompt` | Inline text to judge |
| `--host` | Ollama host URL: this flag, then `$OLLAMA_HOST`, then `http://localhost:11434`; a scheme-less value gets `http://` prefixed |
| `--timeout` | Request timeout (default `2m`) |
| `--verbose` | Print the score, confidence, and probabilities to stderr |

Exactly one of `--image`, `--file`, or `--prompt` must be provided.

## Commands

### `noul` — judge a boolean statement

| Flag | Description |
| --- | --- |
| `--question` | Boolean statement to judge against the input (required) |
| `--threshold` | Score at or above which the statement is judged true, `0` to `1` (default `0.5`) |

```sh
$ ollama-choose noul --model clef-flash:9b --prompt "2+2 equals 4." --question "the state is factually correct"
true

$ ollama-choose noul --model clef-flash:9b --image red.png --question "the image is predominantly red" --verbose
score=0.9753
true
```

(`red.png` is a solid red image.)

The score is the model's probability that the statement is true; `--verbose` prints it. A statement below the threshold prints `false` and exits 1:

```sh
$ ollama-choose noul --model clef-flash:9b --file letter.txt --question "the text is written in English"
false
```

(`letter.txt` contains French text.)

### `choice` — choose one of several options

| Flag | Description |
| --- | --- |
| `--question` | Question to answer with one of the options (required) |
| `--option` | One option, as `key: description` or just `key`; repeat for each option (2 to 26 required) |

```sh
$ ollama-choose choice --model clef-flash:9b \
  --prompt "I was charged twice. Please refund the extra payment." \
  --question "Which team should handle this ticket?" \
  --option "billing: Payments and refunds" \
  --option "technical: Bugs and integrations" \
  --option "other: None of the above"
billing
```

Options are sent in the order given, and ties go to the first option — list your preferred option first. `--verbose` prints the confidence and each option's probability in flag order:

```sh
$ ollama-choose choice --model clef-flash:9b \
  --prompt "I was charged twice. Please refund the extra payment." \
  --question "Which team should handle this ticket?" \
  --option "billing: Payments and refunds" --option "technical: Bugs and integrations" --option "other: None of the above" \
  --verbose
confidence=0.8833
probability billing 0.9764
probability technical 0.0128
probability other 0.0109
billing
```

Images work too:

```sh
$ ollama-choose choice --model clef-flash:9b --image red.png \
  --question "Which color dominates the image?" \
  --option red --option green --option blue
red
```

### `score` — rate on an ordered scale

| Flag | Description |
| --- | --- |
| `--question` | Question to answer with a score on the scale (required) |
| `--level` | One scale level, lowest first; repeat for each level (2 to 26 required) |

```sh
$ ollama-choose score --model clef-flash:9b \
  --prompt "Our checkout has returned 500 errors since 9am." \
  --question "How urgent is this ticket?" \
  --level "Routine: no time pressure" \
  --level "Soon: a customer is inconvenienced" \
  --level "Immediate: a critical service is unavailable"
1.9280781319704718
```

The score is a probability-weighted level index: `0` is the lowest level, `N-1` the highest, so `1.928…` leans strongly toward `Immediate`. `--verbose` prints the confidence and per-level probabilities:

```sh
$ ollama-choose score --model clef-flash:9b \
  --prompt "Our checkout has returned 500 errors since 9am." \
  --question "How urgent is this ticket?" \
  --level "Routine: no time pressure" --level "Soon: a customer is inconvenienced" --level "Immediate: a critical service is unavailable" \
  --verbose
confidence=0.7748
probability 0 0.0167 Routine: no time pressure
probability 1 0.0384 Soon: a customer is inconvenienced
probability 2 0.9448 Immediate: a critical service is unavailable
1.9280781319704718
```

## Exit codes

The commands behave like `grep`: `0` is the desired outcome, `1` a negative answer, `2` an error.

| Command | 0 | 1 | 2 |
| --- | --- | --- | --- |
| `noul` | Judged true | Judged false | Error |
| `choice` | Option chosen | — | Error |
| `score` | Score printed | — | Error |

```sh
if ollama-choose noul --model clef-flash:9b --file ticket.txt --question "the customer requests a refund"; then
  echo "route to billing"
fi
```

## Development

```sh
task check    # fmt-check, vet, build, and test
task test     # unit tests
task cover    # coverage report
task binary   # build ./bin/ollama-choose
task run -- noul --model clef-flash:9b --prompt "hi" --question "a question"   # go run
task clean    # remove build artifacts
```

## License

[MIT](LICENSE)