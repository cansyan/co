package main

import (
	"fmt"
	"log"
	"tui/ui"
)

func main() {
	buttonQuit := ui.Button("Quit")
	statusBar := ui.Text("Status: Ready")
	editor := ui.TextEditor()
	editor.SetText("// Welcome to the Text Editor!\n\n")
	editor.OnChange(func() {
		row, col := editor.Cursor()
		statusBar.SetText(fmt.Sprintf("Line %d, Column %d", row+1, col+1))
	})

	root := ui.VStack(
		ui.HStack(
			ui.Button("file1"),
			ui.Divider(),
			ui.Button("file2"),
			ui.Spacer(),
			ui.Button("New"),
			ui.Button("Open"),
			ui.Button("Close"),
			buttonQuit,
		),
		ui.Divider(),
		ui.Fill(editor),
		ui.Divider(),
		ui.PaddingH(statusBar, 1),
	)

	app := ui.NewApp(ui.Border(root))
	app.Focus(editor)
	buttonQuit.OnClick(func() {
		app.Stop()
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
