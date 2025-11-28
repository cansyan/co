package main

import (
	"fmt"
	"log"
	"tui/ui"
)

func main() {
	statusBar := ui.NewText("Status: Ready")

	logViewer := ui.NewTextViewer("")
	logViewer.OnChange(func() {
		statusBar.SetText(fmt.Sprintf("OffsetY: %d, AutoTail: %t, lines: %d",
			logViewer.OffsetY, logViewer.AutoTail, len(logViewer.Lines)))
	})
	log.SetOutput(logViewer)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	editor := ui.NewTextEditor()
	editor.SetText("// Welcome to the Text Editor!\n\n")
	editor.OnChange(func() {
		row, col := editor.Cursor()
		statusBar.SetText(fmt.Sprintf("Line %d, Column %d", row+1, col+1))
	})

	root := ui.VStack(
		ui.Fill(ui.HStack(
			ui.Fill(editor),
			ui.Divider(),
			ui.Fill(logViewer),
		)),
		ui.Divider(),
		ui.PaddingH(statusBar, 1),
	)

	app := ui.NewApp(ui.Border(root))

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
