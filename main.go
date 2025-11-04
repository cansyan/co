package main

import "tui/ui"

func main() {
	root := ui.VStack(
		ui.Grow(ui.HStack(
			ui.Text("sidebar"),
			ui.Divider(),
			ui.Grow(ui.VStack(
				ui.PaddingH(ui.Text("title"), 1),
				ui.Divider(),
				ui.Grow(ui.PaddingH(ui.Text("content area"), 1)),
			)),
		)),
		ui.Divider(),
		ui.Text("footer"),
	)

	app := ui.NewApp(root)
	if err := app.Run(); err != nil {
		panic(err)
	}
}
