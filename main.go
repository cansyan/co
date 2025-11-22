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

	buttonQuit := ui.NewButton("Quit")
	statusBar := ui.NewText("Status: Ready")
	editor := ui.NewTextEditor()
	editor.SetText("// Welcome to the Text Editor!\n\n")
	editor.OnChange(func() {
		row, col := editor.Cursor()
		statusBar.SetText(fmt.Sprintf("Line %d, Column %d", row+1, col+1))
	})

	sideBar := ui.NewList()
	sideBar.Append("file1.txt", nil)
	sideBar.Append("file2.txt", nil)

	tabs := new(Tabs)
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
	buttonQuit.OnClick = func() {
		app.Stop()
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type Tabs struct {
	ui.BasicElement
	Selected int
	Items    []*Tab
}

type Tab struct {
	Button  *ui.Button
	Content ui.Element
}

func (t *Tabs) Append(name string, e ui.Element) {
	button := ui.NewButton(name)
	button.OnClick = func() {
		for i, tab := range t.Items {
			if tab.Button == button {
				t.Selected = i
			}
		}
	}
	tab := &Tab{
		Button:  button,
		Content: e,
	}
	t.Items = append(t.Items, tab)
}

func (t *Tabs) MinSize() (w, h int) {
	w, h = 0, 0
	for _, tab := range t.Items {
		bw, bh := tab.Button.MinSize()
		w += bw
		if bh > h {
			h = bh
		}
	}
	cw, ch := t.Items[t.Selected].Content.MinSize()
	if cw > w {
		w = cw
	}
	h += ch
	return
}

func (t *Tabs) Layout(x, y, w, h int) *ui.LayoutNode {
	node := &ui.LayoutNode{Element: t, Rect: ui.Rect{X: x, Y: y, W: w, H: h}}
	offset := 0
	for i, tab := range t.Items {
		if i == t.Selected {
			tab.Button.Foreground("blue")
		} else {
			tab.Button.Foreground("") // reset
		}
		bw, _ := tab.Button.MinSize()
		node.Children = append(node.Children, tab.Button.Layout(x+offset, y, bw, 1))
		offset += bw
	}

	node.Children = append(node.Children, t.Items[t.Selected].Content.Layout(x, y+1, w, h-1))
	return node
}

func (t *Tabs) Render(s ui.Screen, rect ui.Rect, style ui.Style) {
	// No-op: children are rendered by the layout node
}
