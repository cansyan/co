package main

import (
	"fmt"
	"log"
	"os"
	"tui/ui"

	"github.com/mattn/go-runewidth"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	f, err := os.OpenFile("/tmp/tui.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.SetOutput(f)

	app := newApp()
	if err := ui.Start(app); err != nil {
		log.Print(err)
		return
	}
}

// app implements ui.Element
type app struct {
	sidebar *ui.TabView
	tabs    []*tab
	active  int
	btnNew  *ui.Button
	btnSave *ui.Button
	btnQuit *ui.Button
	status  *ui.Text
}

func newApp() *app {
	v := &app{
		sidebar: new(ui.TabView),
		btnNew:  ui.NewButton("New"),
		btnSave: ui.NewButton("Save"),
		btnQuit: ui.NewButton("Quit"),
		status:  ui.NewText("Ready"),
	}
	v.appendTab("untitled", "")
	ui.Focus(v)

	v.btnNew.OnClick = func() {
		v.appendTab("untitled", "")
		ui.Focus(v)
	}
	v.btnQuit.OnClick = ui.Stop

	folder := ui.NewListView()
	folder.Append("file1.txt", nil)
	folder.Append("go.mod", func(name string) {
		bs, err := os.ReadFile(name)
		if err != nil {
			log.Print(err)
			return
		}
		v.appendTab(name, string(bs))
	})
	v.sidebar.Append("Folder", folder)
	symbolList := ui.NewListView()
	symbolList.Append("Element", nil)
	symbolList.Append("Text", nil)
	symbolList.Append("TextInput", nil)
	v.sidebar.Append("Symbol", symbolList)
	return v
}

func (a *app) appendTab(label string, content string) {
	editor := ui.NewTextEditor()
	editor.SetText(content)
	editor.OnChange(func() {
		row, col := editor.Cursor()
		a.status.Label = fmt.Sprintf("Line %d, Column %d", row+1, col+1)
	})
	a.tabs = append(a.tabs, &tab{
		av:    a,
		label: label,
		body:  editor,
	})
	a.active = len(a.tabs) - 1
}

func (a *app) deleteTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}

	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	if i < a.active {
		a.active--
	} else if i == a.active {
		a.active = max(0, len(a.tabs)-1)
	}
}

// func (a *app) setActive(i int) {
// 	if len(a.tabs) == 0 || i > len(a.tabs)-1 {
// 		return
// 	}
// 	a.active = i
// }

func (a *app) MinSize() (int, int) {
	var maxW, maxH int
	for _, t := range a.tabs {
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

func (a *app) Layout(x, y, w, h int) *ui.LayoutNode {
	n := &ui.LayoutNode{
		Element: a,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}

	labelView := ui.HStack()
	for i, tab := range a.tabs {
		labelView.Append(tab)
		if i != len(a.tabs)-1 {
			labelView.Append(ui.Divider())
		}
	}
	editorView := ui.VStack(
		ui.HStack(labelView.Grow(), a.btnNew, a.btnSave, a.btnQuit),
	)
	if len(a.tabs) > 0 {
		editorView.Append(ui.Grow(a.tabs[a.active].body))
	}

	view := ui.VStack(
		ui.HStack(
			a.sidebar.Frame(17, 0),
			ui.Divider(),
			editorView.Grow(),
		).Grow(),
		a.status,
	)
	n.Children = append(n.Children, view.Layout(x, y, w, h))
	return n
}

func (a *app) Render(ui.Screen, ui.Rect) {
	// no-op
}

func (a *app) FocusTarget() ui.Element {
	if len(a.tabs) == 0 {
		return a
	}
	return a.tabs[a.active].body
}

func (a *app) OnFocus() {}
func (a *app) OnBlur()  {}

type tab struct {
	av      *app
	label   string
	body    ui.Element
	hovered bool
	style   ui.Style
}

const tabItemWidth = 15

func (t *tab) MinSize() (int, int) { return tabItemWidth, 1 }
func (t *tab) Layout(x, y, w, h int) *ui.LayoutNode {
	return &ui.LayoutNode{
		Element: t,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
}
func (t *tab) Render(screen ui.Screen, r ui.Rect) {
	var st ui.Style
	if t == t.av.tabs[t.av.active] {
		st = t.style.Merge(ui.StyleActiveTab)
	} else if t.hovered {
		st = t.style.Merge(ui.StyleHoverTab)
	}

	format := " %s x "
	labelWidth := tabItemWidth - 4
	if runewidth.StringWidth(t.label) <= labelWidth {
		t.label = runewidth.FillRight(t.label, labelWidth)
	} else {
		t.label = runewidth.Truncate(t.label, labelWidth, "…")
	}
	out := fmt.Sprintf(format, t.label)
	ui.DrawString(screen, r.X, r.Y, r.W, out, st.Apply())
}

func (t *tab) OnMouseDown(lx, ly int) {
	w, _ := t.MinSize()
	// check if pressing charater "x"
	if lx >= w-2 {
		for i, tab := range t.av.tabs {
			if tab == t {
				t.av.deleteTab(i)
				return
			}
		}
		return
	}

	for i, tab := range t.av.tabs {
		if tab == t {
			t.av.active = i
		}
	}
}

func (t *tab) OnMouseUp(lx, ly int) {}
func (t *tab) OnMouseEnter() {
	t.hovered = true
}
func (t *tab) OnMouseLeave() {
	t.hovered = false
}
func (t *tab) OnMouseMove(rx, ry int) {}
func (t *tab) FocusTarget() ui.Element {
	if len(t.av.tabs) == 0 {
		return t.av
	}
	return t.av.tabs[t.av.active].body
}

func (t *tab) OnFocus() {}
func (t *tab) OnBlur()  {}
