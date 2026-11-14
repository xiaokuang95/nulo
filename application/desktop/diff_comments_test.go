package main

import "testing"

func TestLineReviewCommentsAddAndDelete(t *testing.T) {
	a := baseApp()
	s := &session{ID: "s1", Changes: &changeSet{Files: []changeFile{{Path: "main.go"}}}}
	file := s.Changes.Files[0]
	a.beginLineComment(s, file, 4, "12")
	a.commentText = "  rename this field  "
	a.saveLineComment(s, file)
	if a.commentRow != -1 {
		t.Fatalf("draft was not cleared: %+v", a)
	}
	if got := s.Changes.Files[0].Comments; len(got) != 1 || got[0].Text != "rename this field" || got[0].Row != 4 || got[0].Label != "12" {
		t.Fatalf("comments = %+v", got)
	}
	a.beginLineComment(s, file, 8, "16")
	a.commentText = "second"
	a.saveLineComment(s, file)
	if got := len(s.Changes.Files[0].Comments); got != 2 {
		t.Fatalf("comments = %d", got)
	}
	a.deleteLineComment(s, file, 0)
	if got := s.Changes.Files[0].Comments; len(got) != 1 || got[0].Text != "second" {
		t.Fatalf("comments after delete = %+v", got)
	}
}

func TestLineReviewCommentDraftIsPerSessionAndFile(t *testing.T) {
	a := baseApp()
	s1 := &session{ID: "s1", Changes: &changeSet{Files: []changeFile{{Path: "a.go"}}}}
	s2 := &session{ID: "s2", Changes: &changeSet{Files: []changeFile{{Path: "a.go"}}}}
	a.beginLineComment(s1, s1.Changes.Files[0], 1, "1")
	if !a.commentDraftFor(s1, s1.Changes.Files[0]) {
		t.Fatal("draft missing for its own session")
	}
	if a.commentDraftFor(s2, s2.Changes.Files[0]) {
		t.Fatal("draft leaked into another session")
	}
}
