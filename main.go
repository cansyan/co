package main

import (
	"fmt"
	"log"
	"tui/ui"
)

func main() {
	statusBar := ui.NewText("Status: Ready")

	logPanel := ui.NewTextViewer("")
	logPanel.OnChange(func() {
		statusBar.SetText(fmt.Sprintf("OffsetY: %d, AutoTail: %t, lines: %d",
			logPanel.OffsetY, logPanel.AutoTail, len(logPanel.Lines)))
	})
	log.SetOutput(logPanel)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	editor := ui.NewTextEditor()
	editor.SetText("// Welcome to the Text Editor!\n\n")
	editor.OnChange(func() {
		row, col := editor.Cursor()
		statusBar.SetText(fmt.Sprintf("Line %d, Column %d", row+1, col+1))
	})

	tabs := &ui.Tabs{Closable: true}
	tabs.Append("untitled", editor)
	tabs.Append("empty", ui.Empty{})

	root := ui.VStack(
		ui.Grow(tabs, 3),
		ui.Divider(),
		ui.Grow(logPanel, 1),
		ui.Divider(),
		statusBar,
	)

	app := ui.NewApp(ui.PaddingH(root, 1))
	app.Focus(tabs)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
