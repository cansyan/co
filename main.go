package main

import (
	"log"
	"tui/ui"
)

func main() {
	buttonExit := ui.Button("Exit")
	root := ui.VStack(
		ui.Grow(ui.HStack(
			ui.Text("sidebar"),
			ui.Divider(),
			ui.Grow(ui.VStack(
				ui.HStack(
					ui.Button("file1"), ui.Button("x|"),
					ui.Button("file2"), ui.Button("x|"),
					ui.Spacer(),
					buttonExit,
				),
				ui.Divider(),
				ui.Grow(ui.Text("content area")),
			)),
		).Spacing(1)),
		ui.Divider(),
		ui.Text("footer"),
	)

	app := ui.NewApp(root)
	buttonExit.OnClick(func() {
		// panic("Exiting the application")
		app.Stop()
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
