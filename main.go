package main

import (
	"log"
	"tui/ui"
)

func main() {
	input := ui.Input()
	buttonExit := ui.Button("Exit")
	root := ui.VStack(
		ui.Fill(ui.HStack(
			ui.Text("sidebar"),
			ui.Divider(),
			ui.Fill(ui.VStack(
				ui.HStack(
					ui.Button("file1"), ui.Button("x|"),
					ui.Button("file2"), ui.Button("x|"),
					ui.Spacer(),
					buttonExit,
				),
				ui.Divider(),
				input,
			)),
		).Spacing(1)),
		ui.Divider(),
		ui.Text("footer"),
	)

	app := ui.NewApp(root)
	app.Focus(input)
	buttonExit.OnClick(func() {
		app.Stop()
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
