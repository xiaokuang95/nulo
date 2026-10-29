package ai

import (
	"bufio"
	"io"
	"strings"
)

func readSSE(r io.Reader, fn func(event, data string) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var event, data strings.Builder
	flush := func() error {
		d := strings.TrimSuffix(data.String(), "\n")
		e := strings.TrimSpace(event.String())
		event.Reset()
		data.Reset()
		if d == "" && e == "" {
			return nil
		}
		return fn(e, d)
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "event:"); ok {
			event.Reset()
			event.WriteString(strings.TrimSpace(rest))
			continue
		}
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			if strings.HasPrefix(rest, " ") {
				rest = rest[1:]
			}
			data.WriteString(rest)
		}
	}
	if err := flush(); err != nil {
		return err
	}
	return sc.Err()
}
