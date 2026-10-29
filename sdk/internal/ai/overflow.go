package ai

import "strings"

func IsContextOverflow(msg Message) bool {
	if msg.StopReason != StopError {
		return false
	}
	t := strings.ToLower(msg.ErrorMessage + " " + msg.ContentString())
	for _, n := range []string{"context length", "maximum context", "too many tokens", "context_length", "prompt is too long"} {
		if strings.Contains(t, n) {
			return true
		}
	}
	return false
}
