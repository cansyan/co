package main

import (
	"fmt"
	"log"
	"os"
	"tui/ui"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	f, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.SetOutput(f)

	buttonQuit := ui.NewButton("Quit")
	statusBar := ui.NewText("Status: Ready")
	editor := ui.NewTextEditor()
	editor.SetText("// Welcome to the Text Editor!\n\n")
	editor.OnChange(func() {
		row, col := editor.Cursor()
		statusBar.SetText(fmt.Sprintf("Line %d, Column %d", row+1, col+1))
	})

	root := ui.VStack(
		// ui.HStack(
		// 	ui.NewButton("file1"),
		// 	ui.Divider(),
		// 	ui.NewButton("file2"),
		// 	ui.Spacer(),
		// 	ui.NewButton("New"),
		// 	ui.NewButton("Open"),
		// 	ui.NewButton("Close"),
		// 	buttonQuit,
		// ),
		// ui.Divider(),
		// ui.Fill(editor),
		ui.Fill(NewTabs(map[string]ui.Element{
			"tab1": ui.NewText("Welcome to the TUI Application"),
			"tab2": editor,
		})),
		ui.Divider(),
		ui.PaddingH(statusBar, 1),
	)

	app := ui.NewApp(ui.Border(root))
	buttonQuit.OnClick(func() {
		app.Stop()
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type Tabs struct {
	Selected int
	Buttons  []*ui.Button
	Contents []ui.Element
}

func NewTabs(tabs map[string]ui.Element) *Tabs {
	t := &Tabs{}
	for name, content := range tabs {
		button := ui.NewButton(name)
		button.OnClick(func() {
			for i, btn := range t.Buttons {
				if btn == button {
					t.Selected = i
				}
			}
		})
		t.Buttons = append(t.Buttons, button)
		t.Contents = append(t.Contents, content)
	}
	return t
}

func (t *Tabs) MinSize() (w, h int) {
	w, h = 0, 0
	for _, btn := range t.Buttons {
		bw, bh := btn.MinSize()
		w += bw
		if bh > h {
			h = bh
		}
	}
	cw, ch := t.Contents[t.Selected].MinSize()
	if cw > w {
		w = cw
	}
	h += ch
	return
}

func (t *Tabs) Layout(x, y, w, h int) *ui.LayoutNode {
	node := &ui.LayoutNode{Element: t, Rect: ui.Rect{X: x, Y: y, W: w, H: h}}
	offset := 0
	for i, b := range t.Buttons {
		if i == t.Selected {
			b.Foreground("blue")
		} else {
			b.Foreground("") // reset
		}
		bw, _ := b.MinSize()
		node.Children = append(node.Children, b.Layout(x+offset, y, bw, 1))
		offset += bw
	}

	node.Children = append(node.Children, t.Contents[t.Selected].Layout(x, y+1, w, h-1))
	return node
}

func (t *Tabs) Render(s ui.Screen, rect ui.Rect, style ui.Style) {
	// No-op: children are rendered by the layout node
}
