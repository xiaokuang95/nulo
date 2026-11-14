package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *app) rewindDialog(c *ui.Context) {
	p := a.pal()
	selected := rewindPoint{}
	if a.rewindIndex >= 0 && a.rewindIndex < len(a.rewindPoints) {
		selected = a.rewindPoints[a.rewindIndex]
	}
	ui.Modal(c, &a.rewindOpen, func() {
		ui.Text(c, "Rewind to checkpoint").FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, "Restore the thread to just before a user message. The session log keeps the newer branch, so nothing is lost on disk.").
			FontSize(11).LineHeight(1.5).TextColor(p.text4)
		if selected.Snapshot {
			num := selected.SnapshotFiles
			label := fmt.Sprintf("Restore %d changed file(s) from this checkpoint", num)
			if num == 1 {
				label = "Restore 1 changed file from this checkpoint"
			}
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				if a.checkbox(c, a.rewindRestoreFiles, label).Clicked() {
					a.rewindRestoreFiles = !a.rewindRestoreFiles
				}
				ui.Text(c, label).FontSize(11).TextColor(p.text3)
			})
			ui.Text(c, "Only files recorded at this checkpoint are restored; newer untracked files are left alone.").
				Font("monospace").FontSize(9).TextColor(p.text4)
		} else if selected.SnapshotNote != "" {
			ui.Text(c, "File snapshot unavailable · "+selected.SnapshotNote).Font("monospace").FontSize(9).TextColor(p.warn)
		} else {
			ui.Text(c, "No file snapshot for this checkpoint; working-tree changes stay as they are.").
				Font("monospace").FontSize(9).TextColor(p.text4)
		}
		ui.Scroll(c).FillWidth().MaxHeight(340).Children(func() {
			ui.Column(c).FillWidth().Gap(3).Children(func() {
				for i, point := range a.rewindPoints {
					idx := i
					item := point
					row := ui.ButtonBase(c).FillWidth().Height(48).Padding(7, 9).Gap(8).Radius(6).
						Justify(ui.Start).Background(ui.Transparent).Label(item.Label)
					if idx == a.rewindIndex {
						row.Background(p.accentSoft).Border(1, p.accent.Alpha(0.28))
					} else if row.Hovered() {
						row.Background(p.surface2)
					}
					if row.Clicked() {
						a.rewindIndex = idx
						a.rewindRestoreFiles = item.Snapshot
					}
					row.Children(func() {
						a.icon(c, "corner", 13, chooser(idx == a.rewindIndex, p.accent, p.text4))
						ui.Column(c).Grow(1).Gap(1).Children(func() {
							ui.Text(c, item.Label).FontSize(12).TextColor(p.text2).SingleLine()
							ui.Text(c, item.Detail).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
						})
						if idx == a.rewindIndex {
							a.icon(c, "check", 12, p.accent)
						}
					})
				}
			})
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			if a.button(c, "Cancel").Clicked() {
				a.rewindOpen = false
			}
			if a.primaryButton(c, "Restore checkpoint", "corner").Clicked() {
				a.confirmRewind()
			}
		})
	})
}
