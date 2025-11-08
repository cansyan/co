package main

import (
	"log"
	"tui/ui"
)

func main() {
	editor := ui.TextEditor()
	buttonQuit := ui.Button("Quit")
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
		ui.Text("status"),
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
