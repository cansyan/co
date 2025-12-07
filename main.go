package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"tui/ui"

	"slices"

	"github.com/gdamore/tcell/v2"
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

	root := newRoot()
	if len(os.Args) > 1 {
		err := root.openFile(os.Args[1])
		if err != nil {
			log.Print(err)
			return
		}
	} else {
		root.appendTab("untitled", "")
	}

	app := ui.Default()
	app.SetFocusID("root", root)
	app.Focus(root)
	app.BindKey("Ctrl+P", root.showPalatte)
	app.BindKey("Ctrl+W", func() {
		root.deleteTab(root.active)
		app.Focus(root)
	})
	if err := app.Serve(root); err != nil {
		log.Print(err)
		return
	}
}

// root implements ui.Element
type root struct {
	tabs    []*tab
	active  int
	btnNew  *ui.Button
	btnSave *ui.Button
	btnQuit *ui.Button
	status  *ui.Text
}

func newRoot() *root {
	r := &root{
		btnNew:  ui.NewButton("New"),
		btnSave: ui.NewButton("Save"),
		btnQuit: ui.NewButton("Quit"),
		status:  ui.NewText("Ready"),
	}
	r.btnNew.OnClick = func() {
		r.appendTab("untitled", "")
		ui.Default().Focus(r)
	}
	r.btnQuit.OnClick = ui.Default().Close
	return r
}

func (r *root) appendTab(label string, content string) {
	editor := ui.NewTextEditor()
	editor.SetText(content)
	editor.OnChange(func() {
		row, col := editor.Cursor()
		r.status.Label = fmt.Sprintf("Line %d, Column %d", row+1, col+1)
	})
	r.tabs = append(r.tabs, &tab{
		av:    r,
		label: label,
		body:  editor,
	})
	r.active = len(r.tabs) - 1
}

func (r *root) deleteTab(i int) {
	if i < 0 || i >= len(r.tabs) {
		return
	}

	r.tabs = slices.Delete(r.tabs, i, i+1)
	if i < r.active {
		r.active--
	} else if i == r.active {
		r.active = max(0, len(r.tabs)-1)
	}
}

func (r *root) MinSize() (int, int) {
	var maxW, maxH int
	for _, t := range r.tabs {
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

func (r *root) Layout(x, y, w, h int) *ui.LayoutNode {
	n := &ui.LayoutNode{
		Element: r,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}

	labelView := ui.HStack()
	for i, tab := range r.tabs {
		labelView.Append(tab)
		if i != len(r.tabs)-1 {
			labelView.Append(ui.Divider())
		}
	}
	editorView := ui.VStack(
		ui.HStack(labelView.Grow(), r.btnNew, r.btnSave, r.btnQuit),
	)
	if len(r.tabs) > 0 {
		editorView.Append(ui.Grow(r.tabs[r.active].body))
	}

	view := ui.VStack(
		editorView.Grow(),
		ui.Divider(),
		r.status,
	)
	n.Children = append(n.Children, view.Layout(x, y, w, h))
	return n
}

func (r *root) Render(ui.Screen, ui.Rect) {
	// no-op
}

func (r *root) FocusTarget() ui.Element {
	if len(r.tabs) == 0 {
		return r
	}
	return r.tabs[r.active].body
}

func (r *root) OnFocus() {}
func (r *root) OnBlur()  {}

func (r *root) showPalatte() {
	palette := NewPalette()
	palette.Add("Color theme: light", ui.SetLightTheme)
	palette.Add("Color theme: dark", ui.SetDarkTheme)
	palette.Add("New File", func() {
		r.appendTab("untitled", "")
		ui.Default().Focus(r)
	})
	palette.Add("Quit", ui.Default().Close)
	palette.Add("Format", func() {
		log.Print("go fmt")
	})

	w, h := ui.Default().Screen().Size()
	pw := w / 2
	_, ph := palette.MinSize()
	px := (w - pw) / 2
	py := 1
	if py+ph > h {
		ph = h - py
	}
	ui.Default().Overlay(palette, ui.Rect{X: px, Y: py, W: pw, H: ph})
	ui.Default().Focus(palette)
}

func (r *root) openFile(name string) error {
	bs, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	r.appendTab(filepath.Base(name), string(bs))
	return nil
}

type tab struct {
	av      *root
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
		st = t.style.Merge(ui.StyleHover)
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

type Palette struct {
	ui.Style
	cmds []*struct {
		Name   string
		Action func()
	}
	input *ui.TextInput
	list  *ui.ListView
}

func NewPalette() *Palette {
	p := &Palette{
		input: new(ui.TextInput),
		list:  ui.NewListView(),
	}
	p.list.Selected = 0
	p.input.OnChange(func() {
		keyword := p.input.Text()
		p.list.Clear()
		p.list.Selected = 0
		for _, cmd := range p.cmds {
			if keyword == "" || containIgnoreCase(cmd.Name, keyword) {
				p.list.Append(cmd.Name, cmd.Action)
			}
		}
	})
	return p
}

func (p *Palette) Add(name string, action func()) {
	p.cmds = append(p.cmds, &struct {
		Name   string
		Action func()
	}{Name: name, Action: action})
	p.list.Append(name, action)
}

func (p *Palette) MinSize() (int, int) {
	w1, h1 := 30, 1 // input box size
	w2, _ := p.list.MinSize()
	h2 := len(p.cmds)
	return max(w1, w2) + 2, h1 + h2 + 2 // +2 for box border
}

func (p *Palette) Layout(x, y, w, h int) *ui.LayoutNode {
	n := &ui.LayoutNode{
		Element: p,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
	view := ui.VStack(
		p.input,
		ui.Grow(p.list),
	).Border("sliver")
	n.Children = append(n.Children, view.Layout(x, y, w, h))
	return n
}

func (p *Palette) Render(ui.Screen, ui.Rect) {
	// no-op
}

func (p *Palette) HandleKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyESC:
		ui.Default().FocusID("root")
	case tcell.KeyDown:
		p.list.Selected = (p.list.Selected + 1) % len(p.list.Items)
	case tcell.KeyUp:
		n := len(p.list.Items)
		p.list.Selected = (p.list.Selected - 1 + n) % n
	case tcell.KeyEnter:
		if len(p.list.Items) > 0 {
			item := p.list.Items[p.list.Selected]
			item.Action()
			ui.Default().FocusID("root")
		}
	default:
		p.input.HandleKey(ev)
	}
}

func (p *Palette) FocusTarget() ui.Element {
	return p
}
func (p *Palette) OnFocus() { p.input.OnFocus() }
func (p *Palette) OnBlur()  {}

func containIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}
