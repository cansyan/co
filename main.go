package main

import (
	"fmt"
	"io"
	"log"
	"tui/ui"

	"github.com/mattn/go-runewidth"
)

func main() {

	// logPanel := ui.NewTextViewer("")
	// logPanel.OnChange(func() {
	// 	statusBar.SetText(fmt.Sprintf("OffsetY: %d, AutoTail: %t, lines: %d",
	// 		logPanel.OffsetY, logPanel.AutoTail, len(logPanel.Lines)))
	// })
	// log.SetOutput(logPanel)
	// log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(io.Discard)

	statusBar := ui.Text("Status: Ready")

	folder := ui.NewListView()
	folder.Append("x.txt", nil)
	folder.Append("y.txt", nil)

	symbolList := ui.NewListView()
	symbolList.Append("Element", nil)
	symbolList.Append("Text", nil)
	symbolList.Append("TextInput", nil)

	sidebar := &ui.TabView{}
	sidebar.Append("Folder", folder)
	sidebar.Append("Symbol", symbolList)

	tv := newEditorTabView()
	root := ui.VStack(
		ui.HStack(
			sidebar.Frame(17, 0),
			ui.Divider(),
			ui.Grow(tv),
		).Grow(),
		ui.Divider(),
		statusBar,
	)

	app := ui.NewApp(root)
	tv.btnQuit.OnClick = app.Stop
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type editorTabView struct {
	tabs    []*tabItem
	active  int
	btnNew  *ui.ButtonElem
	btnQuit *ui.ButtonElem
}

func newEditorTabView() *editorTabView {
	v := &editorTabView{}
	v.appendTab("untitled", ui.NewTextEditor())

	v.btnNew = ui.Button("New")
	v.btnNew.OnClick = func() {
		v.appendTab("untitled", ui.NewTextEditor())
	}
	v.btnQuit = ui.Button("Quit")
	return v
}

func (v *editorTabView) appendTab(label string, body ui.Element) {
	v.tabs = append(v.tabs, &tabItem{
		label: label,
		body:  body,
	})
}

// func (v *editorTabView) removeTab(i int) {}

func (v *editorTabView) MinSize() (int, int) {
	var maxW, maxH int
	for _, t := range v.tabs {
		w, h := t.body.MinSize()
		if w > maxW {
			maxW = w
		}
		if h > maxH {
			maxH = h
		}
	}
	return maxW, maxH + 1 // +1 for tab label
}

func (v *editorTabView) Layout(x, y, w, h int) *ui.LayoutNode {
	n := &ui.LayoutNode{
		Element: v,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
	if len(v.tabs) == 0 || v.active >= len(v.tabs) {
		return n
	}

	hs := ui.HStack()
	for i, t := range v.tabs {
		hs.Append(t)
		if i != len(v.tabs)-1 {
			hs.Append(ui.Divider())
		}
	}
	n.Children = append(n.Children, ui.HStack(
		hs.Grow(),
		v.btnNew,
		v.btnQuit,
	).Layout(x, y, w, 1))

	n.Children = append(n.Children, v.tabs[v.active].body.Layout(x, y+1, w, h-1))
	return n
}

func (v *editorTabView) Render(ui.Screen, ui.Rect, ui.Style) {
	// no-op
}

type tabItem struct {
	label string
	body  ui.Element
}

const tabItemWidth = 15

func (ti *tabItem) MinSize() (int, int) { return tabItemWidth, 1 }
func (ti *tabItem) Layout(x, y, w, h int) *ui.LayoutNode {
	return &ui.LayoutNode{
		Element: ti,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
}
func (ti *tabItem) Render(screen ui.Screen, r ui.Rect, style ui.Style) {
	format := " %s x "
	labelWidth := tabItemWidth - 4
	if runewidth.StringWidth(ti.label) <= labelWidth {
		ti.label = runewidth.FillRight(ti.label, labelWidth)
	} else {
		ti.label = runewidth.Truncate(ti.label, labelWidth, "…")
	}
	out := fmt.Sprintf(format, ti.label)
	ui.DrawString(screen, r.X, r.Y, r.W, out, style.Apply())
}
