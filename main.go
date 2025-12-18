package main

import (
	"flag"
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

var light = flag.Bool("light", false, "use light color theme")

func main() {
	flag.Parse()
	if *light {
		ui.Theme = ui.NewBreakersTheme()
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	f, err := os.OpenFile("/tmp/tui.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.SetOutput(f)

	root := newRoot()
	if path := flag.Arg(0); path != "" {
		err := root.openFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	} else {
		root.appendTab("untitled", "")
	}

	app := ui.Default()
	// app.SetFocusID("root", root)
	app.Focus(root)
	app.BindKey("Ctrl+P", root.showPalatte)
	app.BindKey("Ctrl+S", root.saveFile)
	app.BindKey("Ctrl+W", func() {
		root.closeTab(root.active)
	})
	app.BindKey("Ctrl+T", func() {
		root.appendTab("untitled", "")
		ui.Default().Focus(root)
	})
	app.BindKey("Esc", func() {
		app.CloseOverlay()
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
		status: ui.NewText("Ready"),
	}
	r.btnNew = ui.NewButton("New", func() {
		r.appendTab("untitled", "")
		ui.Default().Focus(r)
	})
	r.btnSave = ui.NewButton("Save", r.saveFile)
	r.btnQuit = ui.NewButton("Quit", ui.Default().Close)
	return r
}

func (r *root) appendTab(label string, content string) {
	editor := ui.NewTextEditor()
	editor.SetText(content)
	editor.OnChange(func() {
		r.status.Label = editor.Debug()
	})
	r.tabs = append(r.tabs, newTab(r, label, editor))
	r.active = len(r.tabs) - 1
}

// closeTab closes the tab at index i, prompting to save if there are unsaved changes.
func (r *root) closeTab(i int) {
	if i < 0 || i >= len(r.tabs) {
		return
	}
	tab := r.tabs[i]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok || !editor.Dirty {
		r.deleteTab(i)
		ui.Default().Focus(r)
		return
	}

	// Prompt to save changes.
	view := ui.VStack(
		ui.NewText("Save the changes before closing?").PaddingH(1),
		ui.HStack(
			ui.NewButton("Don't Save", func() {
				r.deleteTab(i)
				ui.Default().CloseOverlay()
			}),

			ui.NewButton("Cancel", func() {
				ui.Default().CloseOverlay()
			}).PaddingH(2),

			ui.NewButton("Save", func() {
				if path := tab.label; path != "untitled" {
					err := os.WriteFile(path, []byte(editor.String()), 0644)
					if err != nil {
						log.Print(err)
						r.status.Label = err.Error()
						return
					}
					r.deleteTab(i)
					ui.Default().CloseOverlay()
					return
				}

				sa := NewSaveAs(func(path string) {
					if path == "" {
						return
					}
					err := os.WriteFile(path, []byte(editor.String()), 0644)
					if err != nil {
						log.Print(err)
						r.status.Label = err.Error()
						return
					}
					r.deleteTab(i)
				})
				ui.Default().Overlay(sa, "center")
			}).Background(ui.Theme.Selection),
		).PaddingH(2),
	).Spacing(1).Border()
	ui.Default().Overlay(view, "top")
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

	if len(r.tabs) == 0 {
		ui.Default().Close()
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

func (r *root) OnFocus()                     {}
func (r *root) OnBlur()                      {}
func (r *root) HandleKey(ev *tcell.EventKey) {}

func (r *root) showPalatte() {
	palette := NewPalette()
	palette.Add("Color theme: Breaks", func() {
		ui.Theme = ui.NewBreakersTheme()
	})
	palette.Add("Color theme: Mariana", func() {
		ui.Theme = ui.NewMarianaTheme()
	})
	palette.Add("New File", func() {
		r.appendTab("untitled", "")
		ui.Default().Focus(r)
	})
	palette.Add("Quit", ui.Default().Close)
	ui.Default().Overlay(palette, "top")
}

func (r *root) openFile(name string) error {
	bs, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	r.appendTab(filepath.Base(name), string(bs))
	return nil
}

func (r *root) saveFile() {
	if len(r.tabs) == 0 {
		return
	}
	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}
	if !editor.Dirty {
		ui.Default().Focus(r)
		return
	}

	if path := tab.label; path != "untitled" {
		err := os.WriteFile(path, []byte(editor.String()), 0644)
		if err != nil {
			log.Print(err)
			r.status.Label = err.Error()
			return
		}
		editor.Dirty = false
		ui.Default().Focus(r)
		return
	}

	sa := NewSaveAs(func(path string) {
		if path == "" {
			return
		}
		err := os.WriteFile(path, []byte(editor.String()), 0644)
		if err != nil {
			log.Print(err)
			r.status.Label = err.Error()
			return
		}
		tab.label = path
		editor.Dirty = false
		ui.Default().Focus(r)
	})
	ui.Default().Overlay(sa, "center")
}

type tab struct {
	root     *root
	label    string
	btnClose *ui.Button
	body     ui.Element
	hovered  bool
	style    ui.Style
}

func newTab(root *root, label string, body ui.Element) *tab {
	t := &tab{
		root:  root,
		label: label,
		body:  body,
	}
	t.btnClose = ui.NewButton("x", func() {
		for i, tab := range root.tabs {
			if tab == t {
				t.root.closeTab(i)
				return
			}
		}
	})
	return t
}

const tabItemWidth = 15

func (t *tab) MinSize() (int, int) { return tabItemWidth, 1 }
func (t *tab) Layout(x, y, w, h int) *ui.LayoutNode {
	bw, bh := t.btnClose.MinSize()
	return &ui.LayoutNode{
		Element: t,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
		Children: []*ui.LayoutNode{
			t.btnClose.Layout(x+tabItemWidth-3, y, bw, bh),
		},
	}
}
func (t *tab) Render(screen ui.Screen, r ui.Rect) {
	var st ui.Style
	if t == t.root.tabs[t.root.active] {
		st.FontUnderline = true
		st = t.style.Merge(st)
	} else if t.hovered {
		st.BG = ui.Theme.Hover
		st = t.style.Merge(st)
	}

	format := " %s"
	labelWidth := tabItemWidth - 3 - 1 // minus button and padding
	var label string
	if runewidth.StringWidth(t.label) <= labelWidth {
		label = runewidth.FillRight(t.label, labelWidth)
	} else {
		label = runewidth.Truncate(t.label, labelWidth, "…")
	}
	label = fmt.Sprintf(format, label)
	ui.DrawString(screen, r.X, r.Y, r.W, label, st.Apply())
}

func (t *tab) OnMouseDown(lx, ly int) {
	// like Sublime Text, instant react on clicking tab, not waiting the mouse up
	for i, tab := range t.root.tabs {
		if tab == t {
			t.root.active = i
			ui.Default().Focus(t.root)
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
		input: ui.NewTextInput(),
		list:  ui.NewListView(),
	}
	p.list.Hovered = 0
	p.input.OnChange(func() {
		keyword := p.input.Text()
		p.list.Clear()
		p.list.Hovered = 0
		for _, cmd := range p.cmds {
			if keyword == "" || containIgnoreCase(cmd.Name, keyword) {
				p.list.Append(cmd.Name, func() {
					cmd.Action()
					ui.Default().CloseOverlay()
				})
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
	f := func() {
		action()
		ui.Default().CloseOverlay()
	}
	p.list.Append(name, f)
}

func (p *Palette) MinSize() (int, int) {
	w1, h1 := 30, 1 // input box size
	w2, h2 := p.list.MinSize()
	return max(w1, w2) + 2, h1 + h2 + 2 // +2 for the border
}

func (p *Palette) Layout(x, y, w, h int) *ui.LayoutNode {
	n := &ui.LayoutNode{
		Element: p,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
	view := ui.VStack(
		p.input,
		p.list,
	).Border()
	n.Children = append(n.Children, view.Layout(x, y, w, h))
	return n
}

func (p *Palette) Render(ui.Screen, ui.Rect) {
	// no-op
}

func (p *Palette) HandleKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyESC:
		ui.Default().CloseOverlay()
	case tcell.KeyDown:
		p.list.Hovered = (p.list.Hovered + 1) % len(p.list.Items)
	case tcell.KeyUp:
		n := len(p.list.Items)
		p.list.Hovered = (p.list.Hovered - 1 + n) % n
	case tcell.KeyEnter:
		if len(p.list.Items) > 0 {
			item := p.list.Items[p.list.Hovered]
			if item.Action != nil {
				item.Action()
			}
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

type SaveAs struct {
	child ui.Element
	btnOK *ui.Button
	input *ui.TextInput
}

func NewSaveAs(action func(string)) *SaveAs {
	msg := ui.NewText("Save as: ")
	input := new(ui.TextInput)
	btnCancel := ui.NewButton("Cancel", func() {
		ui.Default().CloseOverlay()
	})
	btnOK := ui.NewButton("OK", func() {
		if action != nil {
			action(input.Text())
		}
		ui.Default().CloseOverlay()
	}).Background(ui.Theme.Selection)

	view := ui.VStack(
		ui.HStack(
			msg,
			ui.Grow(input),
		).PaddingH(1),

		ui.HStack(
			btnCancel,
			ui.Spacer,
			btnOK,
		).PaddingH(4),
	).Spacing(1).Frame(28, 0).Border()

	return &SaveAs{
		child: view,
		btnOK: btnOK,
		input: input,
	}
}

func (m *SaveAs) MinSize() (int, int) {
	return m.child.MinSize()
}

func (m *SaveAs) Layout(x, y, w, h int) *ui.LayoutNode {
	node := &ui.LayoutNode{
		Element: m,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
	node.Children = append(node.Children, m.child.Layout(x, y, w, h))
	return node
}

func (m *SaveAs) Render(s ui.Screen, r ui.Rect) {}

func (m *SaveAs) HandleKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyESC:
		ui.Default().CloseOverlay()
	case tcell.KeyEnter:
		m.btnOK.OnClick()
		ui.Default().CloseOverlay()
	default:
		m.input.HandleKey(ev)
	}
}

func (m *SaveAs) FocusTarget() ui.Element {
	return m
}

func (m *SaveAs) OnFocus() { m.input.OnFocus() }
func (m *SaveAs) OnBlur()  {}
