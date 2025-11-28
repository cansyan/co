package main

import (
	"fmt"
	"log"
	"tui/ui"
)

func main() {
	statusBar := ui.NewText("Status: Ready")

	tv := ui.NewTextViewer("")
	tv.OnChange(func() {
		statusBar.SetText(fmt.Sprintf("OffsetY: %d, AutoTail: %t, lines: %d",
			tv.OffsetY, tv.AutoTail, len(tv.Lines)))
	})
	log.SetOutput(tv)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	editor := ui.NewTextEditor()
	editor.SetText("// Welcome to the Text Editor!\n\n")
	editor.OnChange(func() {
		row, col := editor.Cursor()
		statusBar.SetText(fmt.Sprintf("Line %d, Column %d", row+1, col+1))
	})

	tabs := &ui.Tabs{Closable: true}
	tabs.Append("log", tv)
	tabs.Append("file1.txt", editor)

	root := ui.VStack(
		ui.Fill(tabs),
		ui.Divider(),
		ui.PaddingH(statusBar, 1),
	)

	app := ui.NewApp(root)
	app.Focus(tabs)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
