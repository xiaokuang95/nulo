package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) editingMessage(s *session) bool {
	return s != nil && a.messageEditID != "" && a.messageEditSessionID == s.ID
}

func (a *app) beginMessageEdit(s *session, m *message, index int) {
	if s == nil || m == nil {
		return
	}
	if s.Status == "running" || s.Status == "awaiting" {
		a.pendingToast = "Stop the current turn before editing a message"
		return
	}
	a.messageEditID = m.EntryID
	a.messageEditIndex = index
	a.messageEditSessionID = s.ID
	a.draft = m.Text
	a.attachments = append([]string(nil), m.Attachments...)
	a.focusComposer = true
	a.showPerm, a.showEnv, a.showModel = false, false, false
	a.pendingToast = "Editing message"
}

func (a *app) cancelMessageEdit(clearDraft bool) {
	if a.messageEditID == "" && a.messageEditSessionID == "" {
		return
	}
	a.messageEditID = ""
	a.messageEditSessionID = ""
	a.messageEditIndex = 0
	if clearDraft {
		a.draft = ""
		a.attachments = nil
	}
}

func (a *app) submitMessageEdit(s *session, text string) {
	if s == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		a.pendingToast = "Message is empty"
		return
	}
	if a.host != nil && s.Real {
		if a.messageEditID == "" {
			a.pendingToast = "Message is not saved yet"
			return
		}
		if err := a.host.editMessage(a, s, a.messageEditID, text, a.attachments); err != nil {
			a.pendingToast = "Edit: " + err.Error()
			return
		}
		a.cancelMessageEdit(true)
		a.pendingToast = "Message updated"
		return
	}
	index := a.messageEditIndex
	if index < 0 || index >= len(s.Messages) || s.Messages[index].Role != "user" {
		a.pendingToast = "Message is no longer in this thread"
		return
	}
	s.Messages = s.Messages[:index]
	s.Messages = append(s.Messages, message{Role: "user", Time: "now", Text: text})
	s.Status = "idle"
	s.Plan.Next = "Message edited. Send the next turn to continue."
	a.cancelMessageEdit(true)
	a.pendingToast = "Message updated"
	a.scrollToEnd = true
}

func (a *app) copyMessage(c *ui.Context, m *message) {
	text := strings.TrimSpace(m.Text)
	if m.Role == "assistant" {
		var parts []string
		for _, b := range m.Blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				parts = append(parts, strings.TrimSpace(b.Text))
			}
		}
		text = strings.Join(parts, "\n\n")
	}
	if text == "" {
		a.pendingToast = "Nothing to copy"
		return
	}
	c.Services().WriteClipboard(text)
	a.pendingToast = "Copied message"
}

func (a *app) retryMessage(s *session, m *message) {
	if s == nil || m == nil || m.Role != "assistant" {
		return
	}
	if s.Status == "running" || s.Status == "awaiting" {
		a.pendingToast = "Stop the current turn before retrying"
		return
	}
	if a.host != nil && s.Real {
		if m.EntryID == "" {
			a.pendingToast = "Message is not saved yet"
			return
		}
		if err := a.host.retryMessage(a, s, m.EntryID); err != nil {
			a.pendingToast = "Retry: " + err.Error()
			return
		}
		a.pendingToast = "Retrying response"
		a.scrollToEnd = true
		return
	}
	a.pendingToast = "Retry is available for live SDK sessions"
}

func (a *app) deleteMessage(s *session, m *message, index int) {
	if s == nil || m == nil {
		return
	}
	if s.Status == "running" || s.Status == "awaiting" {
		a.pendingToast = "Stop the current turn before deleting a message"
		return
	}
	label := "Delete this message?"
	if m.Role == "user" {
		label = "Delete this message and everything after it?"
	}
	a.confirmDestructive(label, "The session log keeps the original branch, so you can fork back to it later.", "Delete message", func() {
		if a.host != nil && s.Real {
			if m.EntryID == "" {
				a.pendingToast = "Message is not saved yet"
				return
			}
			if err := a.host.deleteMessage(a, s, m.EntryID); err != nil {
				a.pendingToast = "Delete: " + err.Error()
				return
			}
			a.pendingToast = "Deleted message"
			return
		}
		if index >= 0 && index < len(s.Messages) {
			s.Messages = s.Messages[:index]
			if len(s.Messages) == 0 {
				s.Welcome = true
				s.Status = "idle"
			}
			a.pendingToast = "Deleted message"
		}
	})
}

func (a *app) branchFromMessage(s *session, m *message, index int) {
	if s == nil || m == nil {
		return
	}
	if a.host != nil && s.Real {
		if m.EntryID == "" {
			a.pendingToast = "Message is not saved yet"
			return
		}
		if _, err := a.host.branchFromMessage(a, s, m.EntryID); err != nil {
			a.pendingToast = "Branch: " + err.Error()
			return
		}
		a.pendingToast = "Branched session"
		a.scrollToEnd = true
		return
	}
	a.forkSession(s)
	if child := a.active(); child != nil && index >= 0 && index < len(child.Messages) {
		child.Messages = child.Messages[:index+1]
	}
	a.pendingToast = fmt.Sprintf("Branched from message %d", index+1)
}

func (a *app) messageMenuButton(c *ui.Context, s *session, m *message, index int) {
	if s == nil || m == nil {
		return
	}
	p := a.pal()
	b := ui.ButtonBase(c).Size(18, 18).Radius(4).Label("Message actions").Tooltip("Message actions")
	if b.Hovered() {
		b.Background(p.surface2)
	}
	b.Menu(func(menu *ui.Menu) {
		if menu.Item("Copy message").Chosen() {
			a.copyMessage(c, m)
		}
		if m.Role == "user" {
			if menu.Item("Edit & resend").Chosen() {
				a.beginMessageEdit(s, m, index)
			}
		} else if m.Role == "assistant" {
			if menu.Item("Retry response").Chosen() {
				a.retryMessage(s, m)
			}
		}
		if menu.Item("Branch from here").Chosen() {
			a.branchFromMessage(s, m, index)
		}
		menu.Separator()
		if menu.Item("Delete from here").Chosen() {
			a.deleteMessage(s, m, index)
		}
	})
	b.Children(func() {
		a.icon(c, "more", 13, p.text4)
	})
}
