package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func mergeInProgress(root string) bool {
	if root == "" {
		return false
	}
	out, err := gitOutput(root, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func mergeConflictFiles(root string) []string {
	if root == "" {
		return nil
	}
	out, err := gitOutput(root, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil
	}
	files := make([]string, 0)
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files
}

func (a *app) openMergeConflicts(s *session, root, branch string) {
	if a == nil || root == "" {
		return
	}
	a.mergeConflictSessionID = ""
	if s != nil {
		a.mergeConflictSessionID = s.ID
	}
	a.mergeConflictRoot = root
	a.mergeConflictBranch = branch
	a.mergeConflictFiles = mergeConflictFiles(root)
	a.mergeConflictOpen = true
	a.mergeEditorOpen = false
}

func (a *app) mergeConflictSession() *session {
	for _, s := range a.sessions {
		if s.ID == a.mergeConflictSessionID {
			return s
		}
	}
	return nil
}

func (a *app) refreshMergeConflicts() {
	a.mergeConflictFiles = mergeConflictFiles(a.mergeConflictRoot)
}

func (a *app) mergeConflictFullPath(path string) (string, string, error) {
	root := filepath.Clean(a.mergeConflictRoot)
	clean := filepath.Clean(path)
	if root == "" || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", errors.New("invalid conflict path")
	}
	full := filepath.Join(root, clean)
	if !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", "", errors.New("conflict path escapes the repository")
	}
	return root, clean, nil
}

func (a *app) resolveMergeFile(path string) error {
	root, clean, err := a.mergeConflictFullPath(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "add", "-A", "--", clean)
	if out, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("git add: %s", message)
	}
	a.refreshMergeConflicts()
	return nil
}

func (a *app) completeMerge() error {
	a.refreshMergeConflicts()
	if len(a.mergeConflictFiles) > 0 {
		return errors.New("resolve every conflict first")
	}
	root := a.mergeConflictRoot
	if root == "" || !mergeInProgress(root) {
		return errors.New("no merge is in progress")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "commit", "--no-edit")
	if out, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("git commit: %s", message)
	}
	s := a.mergeConflictSession()
	a.mergeConflictOpen = false
	a.mergeEditorOpen = false
	if s != nil {
		a.addHostNotice(s, "Completed merge of "+a.mergeConflictBranch)
		a.requestChanges(s)
	}
	a.pendingToast = "Merge completed"
	return nil
}

func (a *app) abortMerge() error {
	root := a.mergeConflictRoot
	if root == "" || !mergeInProgress(root) {
		return errors.New("no merge is in progress")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "merge", "--abort")
	if out, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("git merge --abort: %s", message)
	}
	s := a.mergeConflictSession()
	a.mergeConflictOpen = false
	a.mergeEditorOpen = false
	if s != nil {
		a.addHostNotice(s, "Aborted merge of "+a.mergeConflictBranch)
		a.requestChanges(s)
	}
	a.pendingToast = "Merge aborted"
	return nil
}

func gitConflictStage(root string, stage int, path string) (string, bool) {
	out, err := gitOutput(root, "show", ":"+strconv.Itoa(stage)+":"+path)
	if err != nil {
		return "", false
	}
	return string(out), true
}

func containsConflictMarkers(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "<<<<<<<") || strings.HasPrefix(line, ">>>>>>>") ||
			strings.HasPrefix(line, "|||||||") || line == "=======" {
			return true
		}
	}
	return false
}

func binaryText(text string) bool {
	return bytes.IndexByte([]byte(text), 0) >= 0
}

func (a *app) openMergeEditor(path string) {
	root, clean, err := a.mergeConflictFullPath(path)
	if err != nil {
		a.pendingToast = "Conflict: " + err.Error()
		return
	}
	a.mergeEditorPath = clean
	a.mergeEditorBase, a.mergeEditorBaseExists = gitConflictStage(root, 1, clean)
	a.mergeEditorOurs, a.mergeEditorOursExists = gitConflictStage(root, 2, clean)
	a.mergeEditorTheirs, a.mergeEditorTheirsExists = gitConflictStage(root, 3, clean)
	full := filepath.Join(root, clean)
	working, readErr := os.ReadFile(full)
	a.mergeEditorWorking = string(working)
	a.mergeEditorResult = string(working)
	a.mergeEditorChosen = false
	a.mergeEditorDelete = false
	a.mergeEditorBinary = binaryText(a.mergeEditorBase) || binaryText(a.mergeEditorOurs) ||
		binaryText(a.mergeEditorTheirs) || binaryText(a.mergeEditorWorking)
	a.mergeEditorError = ""
	a.mergeEditorTab = 3
	if readErr != nil && !os.IsNotExist(readErr) {
		a.mergeEditorError = readErr.Error()
	}
	a.mergeEditorOpen = true
}

func (a *app) chooseMergeSide(local bool) {
	a.mergeEditorChosen = true
	if local {
		a.mergeEditorResult = a.mergeEditorOurs
		a.mergeEditorDelete = !a.mergeEditorOursExists
	} else {
		a.mergeEditorResult = a.mergeEditorTheirs
		a.mergeEditorDelete = !a.mergeEditorTheirsExists
	}
	a.mergeEditorTab = 3
	a.mergeEditorError = ""
}

func (a *app) saveMergeEditor() {
	if !a.mergeEditorOpen {
		return
	}
	root, clean, err := a.mergeConflictFullPath(a.mergeEditorPath)
	if err != nil {
		a.mergeEditorError = err.Error()
		return
	}
	if a.mergeEditorBinary && !a.mergeEditorChosen {
		a.mergeEditorError = "binary conflict: choose Local or Worktree, or resolve it externally"
		return
	}
	if !a.mergeEditorBinary && containsConflictMarkers(a.mergeEditorResult) {
		a.mergeEditorError = "conflict markers are still present"
		return
	}
	full := filepath.Join(root, clean)
	if a.mergeEditorDelete {
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			a.mergeEditorError = err.Error()
			return
		}
	} else {
		mode := os.FileMode(0o644)
		if info, err := os.Stat(full); err == nil {
			mode = info.Mode().Perm()
		}
		if err := os.WriteFile(full, []byte(a.mergeEditorResult), mode); err != nil {
			a.mergeEditorError = err.Error()
			return
		}
	}
	if err := a.resolveMergeFile(clean); err != nil {
		a.mergeEditorError = err.Error()
		return
	}
	a.mergeEditorOpen = false
	a.mergeEditorError = ""
	a.pendingToast = "Resolved " + clean
}

func (a *app) mergeEditorPane(c *ui.Context, value *string, exists bool, empty string) {
	p := a.pal()
	if !exists {
		ui.Text(c, empty).Font("monospace").FontSize(10).TextColor(p.text4)
		return
	}
	ui.TextAreaBase(c, value).FillWidth().Height(360).Padding(8, 10).Radius(6).
		Border(1, p.lineStrong).Background(p.surface2).Font("monospace").FontSize(10).
		TextColor(p.text).ReadOnly(true)
}

func (a *app) mergeEditorDialog(c *ui.Context) {
	p := a.pal()
	ui.Modal(c, &a.mergeEditorOpen, func() {
		ui.Text(c, "Resolve "+a.mergeEditorPath).Font("monospace").FontSize(16).FontWeight(600).TextColor(p.text).SingleLine()
		ui.Text(c, "Base, Local and Worktree are read-only. Edit Result, or take one side wholesale.").
			FontSize(11).TextColor(p.text4)
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			if a.smallButton(c, chooser(a.mergeEditorTab == 0, "Base ✓", "Base")).Clicked() {
				a.mergeEditorTab = 0
			}
			if a.smallButton(c, chooser(a.mergeEditorTab == 1, "Local ✓", "Local")).Clicked() {
				a.mergeEditorTab = 1
			}
			if a.smallButton(c, chooser(a.mergeEditorTab == 2, "Worktree ✓", "Worktree")).Clicked() {
				a.mergeEditorTab = 2
			}
			if a.smallButton(c, chooser(a.mergeEditorTab == 3, "Result ✓", "Result")).
				Disabled(a.mergeEditorBinary).Clicked() {
				a.mergeEditorTab = 3
			}
		})
		if a.mergeEditorBinary {
			ui.Text(c, "Binary conflict. Choose Local or Worktree, or resolve it in an external editor.").
				FontSize(11).TextColor(p.warn)
		}
		if a.mergeEditorError != "" {
			ui.Text(c, a.mergeEditorError).FontSize(11).TextColor(p.bad)
		}
		switch a.mergeEditorTab {
		case 0:
			a.mergeEditorPane(c, &a.mergeEditorBase, a.mergeEditorBaseExists, "(no base version)")
		case 1:
			a.mergeEditorPane(c, &a.mergeEditorOurs, a.mergeEditorOursExists, "(deleted on the local side)")
		case 2:
			a.mergeEditorPane(c, &a.mergeEditorTheirs, a.mergeEditorTheirsExists, "(deleted on the worktree side)")
		default:
			if a.mergeEditorBinary {
				ui.Text(c, "(binary result hidden)").Font("monospace").FontSize(11).TextColor(p.text4)
			} else {
				ui.TextAreaBase(c, &a.mergeEditorResult).FillWidth().Height(360).Padding(8, 10).Radius(6).
					Border(1, p.lineStrong).Background(p.surface2).Font("monospace").FontSize(10).
					TextColor(p.text).Label("Merge result")
			}
		}
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if a.smallButton(c, "Use local").Clicked() {
				a.chooseMergeSide(true)
			}
			if a.smallButton(c, "Use worktree").Clicked() {
				a.chooseMergeSide(false)
			}
			if a.smallButton(c, "Reload").Clicked() {
				a.openMergeEditor(a.mergeEditorPath)
			}
			ui.Spacer(c)
			if a.button(c, "Cancel").Clicked() {
				a.mergeEditorOpen = false
			}
			if a.primaryButton(c, "Save & resolve", "check").Clicked() {
				a.saveMergeEditor()
			}
		})
	})
}

func (a *app) mergeConflictDialog(c *ui.Context) {
	p := a.pal()
	ui.Modal(c, &a.mergeConflictOpen, func() {
		ui.Text(c, "Merge conflicts").FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, a.mergeConflictBranch+" into local · "+a.mergeConflictRoot).
			Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
		ui.Text(c, "Open each file, resolve the markers, then mark it resolved. Complete the merge when the list is empty.").
			FontSize(11).LineHeight(1.5).TextColor(p.text4)
		if len(a.mergeConflictFiles) == 0 {
			ui.Text(c, "All conflicts are resolved.").FontSize(12).TextColor(p.accent2)
		} else {
			ui.Scroll(c).FillWidth().MaxHeight(300).Children(func() {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					for _, name := range a.mergeConflictFiles {
						file := name
						ui.Row(c).FillWidth().Height(32).Padding(4, 6).Gap(6).AlignItems(ui.Center).
							BorderWidth(0, 0, 1, 0).BorderColor(p.line.Alpha(0.7)).Children(func() {
							ui.Text(c, file).Font("monospace").FontSize(10).TextColor(p.text2).Grow(1).SingleLine()
							if a.smallButton(c, "Resolve").Clicked() {
								a.openMergeEditor(file)
							}
							if a.smallButton(c, "Open").Clicked() {
								if err := mygo.Shell.OpenPath(filepath.Join(a.mergeConflictRoot, file)); err != nil {
									a.pendingToast = "Open: " + err.Error()
								}
							}
							if a.smallButton(c, "Mark resolved").Clicked() {
								if err := a.resolveMergeFile(file); err != nil {
									a.pendingToast = "Conflict: " + err.Error()
								}
							}
						})
					}
				})
			})
		}
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if a.smallButton(c, "Open repo").Clicked() {
				if err := mygo.Shell.OpenPath(a.mergeConflictRoot); err != nil {
					a.pendingToast = "Open: " + err.Error()
				}
			}
			if a.smallButton(c, "Refresh").Clicked() {
				a.refreshMergeConflicts()
			}
			if a.dangerButton(c, "Abort merge").Clicked() {
				if err := a.abortMerge(); err != nil {
					a.pendingToast = "Merge: " + err.Error()
				}
			}
			ui.Spacer(c)
			if a.button(c, "Close").Clicked() {
				a.mergeConflictOpen = false
				a.mergeEditorOpen = false
			}
			if a.primaryButton(c, "Complete merge", "check").Disabled(len(a.mergeConflictFiles) > 0).Clicked() {
				if err := a.completeMerge(); err != nil {
					a.pendingToast = "Merge: " + err.Error()
				}
			}
		})
	})
}
