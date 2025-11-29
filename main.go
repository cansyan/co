package main

import (
	"fmt"
	"io"
	"log"
	"tui/ui"
)

func main() {
	statusBar := ui.Text("Status: Ready")

	// logPanel := ui.NewTextViewer("")
	// logPanel.OnChange(func() {
	// 	statusBar.SetText(fmt.Sprintf("OffsetY: %d, AutoTail: %t, lines: %d",
	// 		logPanel.OffsetY, logPanel.AutoTail, len(logPanel.Lines)))
	// })
	// log.SetOutput(logPanel)
	// log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(io.Discard)

	editor := ui.NewTextEditor()
	editor.SetText("// Welcome to the Text Editor!\n\n")
	editor.OnChange(func() {
		row, col := editor.Cursor()
		statusBar.SetText(fmt.Sprintf("Line %d, Column %d", row+1, col+1))
	})

	folder := ui.NewListView()
	folder.Append("x.txt", func() { editor.SetText("xxxxx") })
	folder.Append("y.txt", func() { editor.SetText("yyyyy") })

	symbolList := ui.NewListView()
	symbolList.Append("Element", nil)
	symbolList.Append("Text", nil)
	symbolList.Append("TextInput", nil)

	sidebar := &ui.Tabs{}
	sidebar.Append("Folder", folder)
	sidebar.Append("Symbol", symbolList)

	tabbar := ui.HStack(
		ui.Text(" untitled "), ui.Divider(), ui.Text(" untitled "),
		ui.Spacer, ui.Button("New"), ui.Button("Save"), ui.Button("Quit"),
	)

	root := ui.VStack(
		ui.HStack(
			sidebar.Grow(1),
			ui.Divider(),
			ui.VStack(
				tabbar,
				editor.Grow(),
			).Grow(5),
		).Grow(),
		ui.Divider(),
		statusBar,
	)

	app := ui.NewApp(root)
	app.Focus(sidebar)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
