package main

import (
	"fmt"
	"log"
	"tui/ui"
)

func main() {
	// log.SetFlags(log.LstdFlags | log.Lshortfile)
	// f, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer f.Close()
	// log.SetOutput(f)

	sideBar := ui.NewListView()
	sideBar.Append("file1.txt", nil)
	sideBar.Append("file2.txt", nil)

	statusBar := ui.NewText("Status: Ready")
	editor := ui.NewTextEditor()
	editor.SetText("// Welcome to the Text Editor!\n\n")
	editor.OnChange(func() {
		row, col := editor.Cursor()
		statusBar.SetText(fmt.Sprintf("Line %d, Column %d", row+1, col+1))
	})

	tabs := &ui.TabView{Closable: true}
	tabs.Append("tab1", editor)
	tabs.Append("tab2", ui.NewText("demo..."))

	root := ui.VStack(
		ui.Fill(ui.HStack(
			sideBar,
			ui.Divider(),
			ui.Fill(tabs),
		)),
		ui.Divider(),
		ui.PaddingH(statusBar, 1),
	)

	app := ui.NewApp(ui.Border(root))
	app.Focus(tabs)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
