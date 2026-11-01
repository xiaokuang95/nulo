package codemode

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// OptionsPrefix starts the optional first line of a script, matching PiG's
// codemode.CodemodeOptionsPrefix.
const OptionsPrefix = "// @options:"

const (
	supportedFields = "`max_output_tokens` and `timeout_ms`"
	// maxTimeoutMs is the largest delay a timer supports, and bounds timeout_ms.
	maxTimeoutMs = 2_147_483_647
)

// SourceOptions are the fields of the options line.
type SourceOptions struct {
	// MaxOutputTokens is the token budget for the script's output; nil means unset.
	MaxOutputTokens *int64
	// TimeoutMs is the hard deadline for the whole script; nil means unset.
	TimeoutMs *int64
}

// ParsedSource is a script with its options line split off.
type ParsedSource struct {
	Code    string
	Options SourceOptions
}

// SourceError reports empty input or invalid options, the way PiG's
// CodemodeSourceError does.
type SourceError struct{ Message string }

func (e *SourceError) Error() string { return e.Message }

// Name is the error's `name` in the reference implementation.
func (e *SourceError) Name() string { return "CodemodeSourceError" }

func sourceErrorf(format string, args ...any) error {
	return &SourceError{Message: fmt.Sprintf(format, args...)}
}

// ParseSource splits the optional first-line `// @options: {...}` from the
// script. The options line is replaced by nothing while its newline stays, so
// line numbers in errors keep matching the input.
func ParseSource(input string) (ParsedSource, error) {
	if strings.TrimSpace(input) == "" {
		return ParsedSource{}, sourceErrorf(
			"Expected JavaScript source text (non-empty). Provide JS only, optionally with a first line `// @options: {\"max_output_tokens\": 1000}`.")
	}
	lines := strings.Split(input, "\n")
	for index, line := range lines[1:] {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), OptionsPrefix) {
			return ParsedSource{}, sourceErrorf(
				"The %s line must be the first line of the input; found one on line %d", OptionsPrefix, index+2)
		}
	}

	newline := strings.IndexByte(input, '\n')
	firstLine := strings.TrimSuffix(lines[0], "\r")
	trimmed := strings.TrimLeft(firstLine, " \t")
	if !strings.HasPrefix(trimmed, OptionsPrefix) {
		return ParsedSource{Code: input}, nil
	}
	code := ""
	if newline != -1 {
		code = input[newline:]
	}
	if strings.TrimSpace(code) == "" {
		return ParsedSource{}, sourceErrorf("The @options line must be followed by JavaScript source on subsequent lines")
	}
	options, err := parseOptions(strings.TrimSpace(trimmed[len(OptionsPrefix):]))
	if err != nil {
		return ParsedSource{}, err
	}
	return ParsedSource{Code: code, Options: options}, nil
}

func parseOptions(directive string) (SourceOptions, error) {
	if directive == "" {
		return SourceOptions{}, sourceErrorf("@options must be a JSON object with supported fields %s", supportedFields)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(directive), &fields); err != nil {
		return SourceOptions{}, sourceErrorf("@options must be valid JSON with supported fields %s: %v", supportedFields, err)
	}
	if fields == nil {
		return SourceOptions{}, sourceErrorf("@options must be a JSON object with supported fields %s", supportedFields)
	}
	for key := range fields {
		if key != "max_output_tokens" && key != "timeout_ms" {
			return SourceOptions{}, sourceErrorf("@options only supports %s; got `%s`", supportedFields, key)
		}
	}

	var options SourceOptions
	if raw, ok := fields["max_output_tokens"]; ok {
		n, ok := safeInteger(raw)
		if !ok {
			return SourceOptions{}, sourceErrorf("@options field `max_output_tokens` must be a non-negative safe integer")
		}
		options.MaxOutputTokens = &n
	}
	if raw, ok := fields["timeout_ms"]; ok {
		n, ok := safeInteger(raw)
		if !ok || n == 0 || n > maxTimeoutMs {
			return SourceOptions{}, sourceErrorf("@options field `timeout_ms` must be a positive integer up to %d", int64(maxTimeoutMs))
		}
		options.TimeoutMs = &n
	}
	return options, nil
}

// safeInteger accepts a JSON number that is a non-negative safe integer, the way
// Number.isSafeInteger does.
func safeInteger(raw json.RawMessage) (int64, bool) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	if f != math.Trunc(f) || f < 0 || f > 1<<53-1 {
		return 0, false
	}
	return int64(f), true
}
