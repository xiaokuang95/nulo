package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// Info is the picker summary of one session JSONL.
type Info struct {
	Path              string    `json:"path"`
	ID                string    `json:"id"`
	CWD               string    `json:"cwd"`
	Name              string    `json:"name"`
	ParentSessionPath string    `json:"parentSessionPath"`
	Created           time.Time `json:"created"`
	Modified          time.Time `json:"modified"`
	MessageCount      int       `json:"messageCount"`
	FirstMessage      string    `json:"firstMessage"`
	AllMessagesText   string    `json:"allMessagesText"`
}

const noMessagesText = "(no messages)"

// List returns the valid session files in dir, newest activity first. Files
// without a session header are skipped, as Pi's session picker does.
func List(dir string) ([]Info, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Info{}, nil
		}
		return nil, err
	}
	infos := []Info{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := Summarize(path)
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	sort.SliceStable(infos, func(i, j int) bool {
		return infos[i].Modified.After(infos[j].Modified)
	})
	return infos, nil
}

// Summarize reads one session file's picker metadata.
func Summarize(path string) (Info, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return Info{}, err
	}
	records, _, err := readSessionRecords(path)
	if err != nil {
		return Info{}, err
	}
	info := Info{Path: path, FirstMessage: noMessagesText}
	var created time.Time
	var modified time.Time
	var texts []string
	haveHeader := false
	for _, raw := range records {
		if !haveHeader {
			var probe struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(raw, &probe) != nil || probe.Type != "session" {
				return Info{}, os.ErrInvalid
			}
			var header Header
			if err := json.Unmarshal(raw, &header); err != nil {
				return Info{}, err
			}
			haveHeader = true
			info.ID = header.ID
			info.CWD = header.CWD
			info.ParentSessionPath = header.ParentSession
			created, _ = time.Parse(time.RFC3339Nano, header.Timestamp)
			continue
		}
		var e Entry
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		switch e.Type {
		case "session_info":
			info.Name = strings.TrimSpace(e.Name)
		case "message":
			if e.Message == nil {
				continue
			}
			info.MessageCount++
			role := e.Message.Role
			if role != "user" && role != "assistant" {
				continue
			}
			at := time.Time{}
			if e.Message.Timestamp > 0 {
				at = time.UnixMilli(e.Message.Timestamp)
			} else if parsed, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
				at = parsed
			}
			if at.After(modified) {
				modified = at
			}
			text := ai.MessageText(*e.Message)
			if text == "" {
				continue
			}
			texts = append(texts, text)
			if info.FirstMessage == noMessagesText && role == "user" {
				info.FirstMessage = text
			}
		}
	}
	if !haveHeader {
		return Info{}, os.ErrInvalid
	}
	info.Created = created
	if !modified.IsZero() {
		info.Modified = modified
	} else if !created.IsZero() {
		info.Modified = created
	} else {
		info.Modified = stat.ModTime()
	}
	info.AllMessagesText = strings.Join(texts, " ")
	return info, nil
}

// Rename records a new session name in path.
func Rename(path, name string) error {
	m, err := Open(path)
	if err != nil {
		return err
	}
	_, err = m.AppendSessionInfo(name)
	return err
}

// Delete removes a session file. A file that is already gone is not an error.
func Delete(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
