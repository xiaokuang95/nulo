package main

import "strings"

func (a *app) commentKeyFor(s *session, file changeFile) string {
	if s == nil {
		return ""
	}
	return s.ID + "\x00" + file.Path
}

func (a *app) beginLineComment(s *session, file changeFile, row int, label string) {
	if s == nil {
		return
	}
	a.commentFile = file.Path
	a.commentRow = row
	a.commentLabel = label
	a.commentText = ""
	a.commentKey = a.commentKeyFor(s, file)
}

func (a *app) cancelLineComment() {
	a.commentFile = ""
	a.commentRow = -1
	a.commentLabel = ""
	a.commentText = ""
	a.commentKey = ""
}

func (a *app) commentDraftFor(s *session, file changeFile) bool {
	if s == nil || a.commentRow < 0 {
		return false
	}
	return a.commentKey == a.commentKeyFor(s, file)
}

func (a *app) saveLineComment(s *session, file changeFile) {
	text := strings.TrimSpace(a.commentText)
	if text == "" || s == nil || s.Changes == nil {
		return
	}
	for i := range s.Changes.Files {
		item := &s.Changes.Files[i]
		if item.Path != file.Path {
			continue
		}
		label := a.commentLabel
		if label == "" {
			label = "diff"
		}
		item.Comments = append(item.Comments, reviewComment{
			Row: a.commentRow, Label: label, Text: text, Time: "now",
		})
		break
	}
	a.cancelLineComment()
	a.pendingToast = "Review note added"
}

func (a *app) deleteLineComment(s *session, file changeFile, index int) {
	if s == nil || s.Changes == nil {
		return
	}
	for i := range s.Changes.Files {
		item := &s.Changes.Files[i]
		if item.Path != file.Path || index < 0 || index >= len(item.Comments) {
			continue
		}
		item.Comments = append(item.Comments[:index], item.Comments[index+1:]...)
		break
	}
	if a.commentFile == file.Path {
		a.cancelLineComment()
	}
	a.pendingToast = "Review note deleted"
}

func commentsForRow(file changeFile, row int) []reviewComment {
	var out []reviewComment
	for _, comment := range file.Comments {
		if comment.Row == row {
			out = append(out, comment)
		}
	}
	return out
}
