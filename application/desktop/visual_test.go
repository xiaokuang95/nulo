package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeScreenshot(t *testing.T) {
	path := os.Getenv("NULO_SCREENSHOT")
	if path == "" {
		t.Skip("set NULO_SCREENSHOT to write a capture")
	}
	state := os.Getenv("NULO_STATE")
	a := newFixtureApp()
	w, h := 1440, 900
	if state == "narrow" || state == "narrowdock" {
		w, h = 1000, 800
	}
	if state == "compact" || state == "narrowside" {
		w, h = 880, 760
	}
	if state == "mid" {
		w, h = 1280, 800
	}
	if state == "wide" {
		w, h = 1728, 960
	}
	if state == "threshold" {
		w, h = 1180, 800
	}
	tt := ui.NewTester(a.view, w, h)
	tt.SetTitleBar(ui.TitleBar{})

	switch state {
	case "palette":
		tt.Key(ui.Cmd, ui.KeyK)
	case "settings":
		tt.Key(ui.Cmd, ui.KeyComma)
	case "appearance":
		tt.Key(ui.Cmd, ui.KeyComma)
		if err := tt.Click("Appearance"); err != nil {
			t.Fatal(err)
		}
	case "paper":
		a.setTheme(true)
	case "new":
		if err := tt.Click("New session"); err != nil {
			t.Fatal(err)
		}
	case "plan":
		if err := tt.Click("Plan"); err != nil {
			t.Fatal(err)
		}
	case "terminal":
		if err := tt.Click("Terminal"); err != nil {
			t.Fatal(err)
		}
	case "context":
		if err := tt.Click("Context"); err != nil {
			t.Fatal(err)
		}
	case "approved":
		if err := tt.Click("Allow once"); err != nil {
			t.Fatal(err)
		}
	case "model":
		a.showModel = true
	case "rewind":
		a.rewindOpen = true
		a.rewindSessionID = a.activeID
		a.rewindPoints = []rewindPoint{
			{Label: "Before: review the approval flow", Detail: "12m · 3 entries after", EntryID: "e3", SnapshotKey: "root", Snapshot: true, SnapshotFiles: 3},
			{Label: "Before: render the plan panel", Detail: "18m · 7 entries after", EntryID: "e1", SnapshotKey: "e1", Snapshot: true, SnapshotFiles: 1},
		}
		a.rewindIndex = 0
		a.rewindRestoreFiles = true
	case "rename":
		a.renameOpen = true
		a.renameID = "s1"
		a.renameValue = "Desktop shell for nulo"
	case "attachments":
		a.attachments = []string{"/tmp/notes.md", "/tmp/spec.pdf"}
	case "projecthover":
		tt.Move(268, 525)
	case "projectnew":
		if err := tt.Click("New session in DOTFILES"); err != nil {
			t.Fatal(err)
		}
	case "collapsed":
		if err := tt.Click("Toggle sidebar · ⌘B"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	case "collapsedgroup":
		if err := tt.Click("NULO"); err != nil {
			t.Fatal(err)
		}
	case "slash":
		if err := tt.Click("Message nulo"); err != nil {
			t.Fatal(err)
		}
		tt.Type("/")
	case "mention":
		if err := tt.Click("Message nulo"); err != nil {
			t.Fatal(err)
		}
		tt.Type("@")
	case "commit":
		if err := tt.Click("Stage all"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Commit…"); err != nil {
			t.Fatal(err)
		}
	case "narrowdock":
		if err := tt.Click("Changes"); err != nil {
			t.Fatal(err)
		}
	case "narrowside":
		if err := tt.Click("Toggle sidebar · ⌘B"); err != nil {
			t.Fatal(err)
		}
	}
	tt.Frame()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
