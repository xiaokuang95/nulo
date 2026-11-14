package main

import (
	"strings"

	"github.com/egoist/mygo"
)

func (a *app) notifyCompletion(s *session) {
	if !a.notifyDone || a.shuttingDown || s == nil || a.win == nil {
		return
	}
	if a.win.IsFocused() || !mygo.NotificationsSupported() {
		return
	}
	title := "Turn complete"
	body := s.Title
	switch s.Status {
	case "awaiting":
		title = "Approval needed"
		body = s.Title + " · a command is waiting for you"
	case "error":
		title = "Turn failed"
		body = s.Title + " · open nulo for details"
	case "paused":
		title = "Turn stopped"
		body = s.Title
	default:
		if strings.TrimSpace(body) == "" {
			body = "nulo finished the current turn"
		}
	}
	note := mygo.NewNotification(mygo.NotificationOptions{
		ID:       "nulo-" + s.ID,
		Title:    "nulo · " + title,
		Body:     body,
		Group:    "nulo",
		Subtitle: s.modelName(),
	})
	note.OnClick(func() {
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			a.selectSession(s.ID)
			a.win.Focus()
		})
	})
	go func() { _ = note.Show() }()
}
