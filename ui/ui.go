// Package ui provides a simple terminal UI toolkit built on top of tcell.
package ui

import (
	"github.com/gdamore/tcell/v2"
)

// ---------------------------------------------------------------------
// PUBLIC API – Import and use these
// ---------------------------------------------------------------------

type Screen = tcell.Screen
type EventKey = tcell.EventKey
type EventMouse = tcell.EventMouse
type EventResize = tcell.EventResize
type Color = tcell.Color

type Element interface {
	Render(s Screen, x, y, width, height int, parent Style)
	MinSize() (w, h int)
	Contains(px, py, x, y, w, h int) bool
}

type Focuser interface {
	Focus()
	Unfocus()
	IsFocused() bool
}

// ---------------------------------------------------------------------
// 1. STYLE
// ---------------------------------------------------------------------

type Style struct {
	FG       Color
	BG       Color
	Bold     bool
	Italic   bool
	Reversed bool
}

var DefaultStyle = Style{FG: tcell.ColorDefault, BG: tcell.ColorDefault}

func (s Style) Apply() tcell.Style {
	st := tcell.StyleDefault
	if s.FG != tcell.ColorDefault {
		st = st.Foreground(s.FG)
	}
	if s.BG != tcell.ColorDefault {
		st = st.Background(s.BG)
	}
	if s.Bold {
		st = st.Bold(true)
	}
	if s.Italic {
		st = st.Italic(true)
	}
	if s.Reversed {
		st = st.Reverse(true)
	}
	return st
}

func mergeStyle(parent, child Style) Style {
	if child.FG == tcell.ColorDefault {
		child.FG = parent.FG
	}
	if child.BG == tcell.ColorDefault {
		child.BG = parent.BG
	}
	child.Bold = child.Bold || parent.Bold
	child.Italic = child.Italic || parent.Italic
	child.Reversed = child.Reversed || parent.Reversed
	return child
}

// ---------------------------------------------------------------------
// 2. WIDGETS
// ---------------------------------------------------------------------

type text struct {
	content string
	style   Style
}

func Text(c string) *text { return &text{content: c, style: DefaultStyle} }

func (t *text) Bold() *text   { t.style.Bold = true; return t }
func (t *text) Italic() *text { t.style.Italic = true; return t }
func (t *text) Foreground(c string) *text {
	t.style.FG = tcell.ColorNames[c]
	return t
}
func (t *text) Background(c string) *text {
	t.style.BG = tcell.ColorNames[c]
	return t
}

func (t *text) MinSize() (int, int) { return len(t.content), 1 }

func (t *text) Render(s Screen, x, y, width, height int, parent Style) {
	st := mergeStyle(parent, t.style)
	for i, r := range t.content {
		if i >= width {
			break
		}
		s.SetContent(x+i, y, r, nil, st.Apply())
	}
}
func (t *text) Contains(px, py, x, y, w, h int) bool { return rectContains(px, py, x, y, w, h) }

type button struct {
	label   string
	style   Style
	onClick func()
}

// Button creates a new button element with the given label.
// By default, the button has no border.
func Button(label string) *button {
	return &button{label: label, style: DefaultStyle}
}
func (b *button) Bold() *button { b.style.Bold = true; return b }
func (b *button) Foreground(c string) *button {
	b.style.FG = tcell.ColorNames[c]
	return b
}
func (b *button) Background(c string) *button {
	b.style.BG = tcell.ColorNames[c]
	return b
}
func (b *button) OnClick(fn func()) *button { b.onClick = fn; return b }

func (b *button) MinSize() (int, int) { return len(b.label) + 2, 1 }

func (b *button) Render(s Screen, x, y, width, height int, parent Style) {
	st := mergeStyle(parent, b.style)
	label := " " + b.label + " "
	cx := x + (width-len(label))/2
	for i, r := range label {
		if cx+i >= x+width {
			break
		}
		s.SetContent(cx+i, y+1, r, nil, st.Apply())
	}
}
func (b *button) Contains(px, py, x, y, w, h int) bool { return rectContains(px, py, x, y, w, h) }

type divider struct {
	vertical bool
	style    Style
}

// Divider creates a horizontal or vertical divider line.
// Should be used inside HStack or VStack.
func Divider() *divider {
	return &divider{style: DefaultStyle}
}
func (d *divider) Foreground(c string) *divider {
	d.style.FG = tcell.ColorNames[c]
	return d
}
func (d *divider) Background(c string) *divider {
	d.style.BG = tcell.ColorNames[c]
	return d
}

func (d *divider) MinSize() (w, h int) { return 1, 1 }
func (d *divider) Render(s Screen, x, y, width, height int, parent Style) {
	st := mergeStyle(parent, d.style)
	if !d.vertical {
		for i := range width {
			s.SetContent(x+i, y+height-1, '-', nil, st.Apply())
			// s.SetContent(x+i, y+height-1, '─', nil, st.Apply())
		}
	} else {
		for i := range height {
			s.SetContent(x+width-1, y+i, '|', nil, st.Apply())
			// s.SetContent(x+width-1, y+i, '│', nil, st.Apply())
		}
	}
}
func (d *divider) Contains(px, py, x, y, w, h int) bool { return false }

// ---------------------------------------------------------------------
// Layouts
// ---------------------------------------------------------------------

type vstack struct {
	children []Element
	style    Style
}

func VStack(children ...Element) *vstack {
	return &vstack{children: children, style: DefaultStyle}
}
func (v *vstack) Foreground(color string) *vstack {
	v.style.FG = tcell.ColorNames[color]
	return v
}
func (v *vstack) Background(color string) *vstack {
	v.style.BG = tcell.ColorNames[color]
	return v
}
func (v *vstack) MinSize() (int, int) {
	maxW, totalH := 0, 0
	for _, child := range v.children {
		cw, ch := child.MinSize()
		if cw > maxW {
			maxW = cw
		}
		totalH += ch
	}
	return maxW, totalH
}
func (v *vstack) Render(s Screen, x, y, width, height int, parent Style) {
	st := mergeStyle(parent, v.style)

	// First pass: measure all children’s min sizes
	totalMinHeight := 0
	growCount := 0
	for _, child := range v.children {
		_, ch := child.MinSize()
		totalMinHeight += ch
		if _, ok := child.(*grow); ok {
			growCount++
		}
	}

	// Compute remaining space
	extra := max(height-totalMinHeight, 0)

	// Second pass: render children with adjusted heights
	used := 0
	for _, child := range v.children {
		if div, ok := child.(*divider); ok {
			div.vertical = false
		}
		_, ch := child.MinSize()
		if _, ok := child.(*grow); ok && growCount > 0 {
			ch += extra / growCount
		}
		if used+ch > height {
			ch = height - used
		}
		if ch > 0 {
			child.Render(s, x, y+used, width, ch, st)
		}
		used += ch
	}
}
func (v *vstack) Contains(px, py, x, y, w, h int) bool {
	used := 0
	for _, child := range v.children {
		_, ch := child.MinSize()
		if used+ch > h {
			ch = h - used
		}
		if ch > 0 && child.Contains(px, py, x, y+used, w, ch) {
			return true
		}
		used += ch
	}
	return false
}

func (v *vstack) Add(e Element) *vstack { v.children = append(v.children, e); return v }

type hstack struct {
	children []Element
	style    Style
}

func HStack(children ...Element) *hstack {
	return &hstack{children: children, style: DefaultStyle}
}
func (h *hstack) MinSize() (int, int) {
	totalW, maxH := 0, 0
	for _, child := range h.children {
		cw, ch := child.MinSize()
		totalW += cw
		if ch > maxH {
			maxH = ch
		}
	}
	return totalW, maxH
}
func (h *hstack) Render(s Screen, x, y, width, height int, parent Style) {
	st := mergeStyle(parent, h.style)

	// First pass: measure children
	totalMinWidth := 0
	growCount := 0
	for _, child := range h.children {
		cw, _ := child.MinSize()
		totalMinWidth += cw
		if _, ok := child.(*grow); ok {
			growCount++
		}
	}

	// Compute remaining width
	extra := max(width-totalMinWidth, 0)

	// Second pass: layout children
	used := 0
	for _, child := range h.children {
		if div, ok := child.(*divider); ok {
			div.vertical = true
		}
		cw, _ := child.MinSize()
		if _, ok := child.(*grow); ok && growCount > 0 {
			cw += extra / growCount
		}
		if used+cw > width {
			cw = width - used
		}
		if cw > 0 {
			child.Render(s, x+used, y, cw, height, st)
		}
		used += cw
	}
}
func (h *hstack) Contains(px, py, x, y, w, height int) bool {
	used := 0
	for _, child := range h.children {
		cw, _ := child.MinSize()
		if used+cw > w {
			cw = w - used
		}
		if cw > 0 && child.Contains(px, py, x+used, y, cw, height) {
			return true
		}
		used += cw
	}
	return false
}
func (h *hstack) Foreground(color string) *hstack {
	h.style.FG = tcell.ColorNames[color]
	return h
}
func (h *hstack) Background(color string) *hstack {
	h.style.BG = tcell.ColorNames[color]
	return h
}

func (h *hstack) Add(e Element) *hstack { h.children = append(h.children, e); return h }

type grow struct {
	child Element
}

// Grow creates a layout element that expands to fill available space.
// Should be used inside HStack or VStack.
func Grow(child Element) *grow {
	return &grow{child: child}
}

func (g *grow) MinSize() (int, int) {
	return g.child.MinSize()
}

func (g *grow) Render(s Screen, x, y, width, height int, parent Style) {
	g.child.Render(s, x, y, width, height, parent)
}

func (g *grow) Contains(px, py, x, y, w, h int) bool {
	return g.child.Contains(px, py, x, y, w, h)
}

type padding struct {
	child                    Element
	top, right, bottom, left int
}

// Padding creates a layout element that adds padding around its child.
func Padding(child Element, p int) *padding {
	return &padding{
		child:  child,
		top:    p,
		right:  p,
		bottom: p,
		left:   p,
	}
}

// PaddingH creates a layout element that adds horizontal padding around its child.
func PaddingH(child Element, p int) *padding {
	return &padding{
		child: child,
		right: p,
		left:  p,
	}
}

// PaddingV creates a layout element that adds vertical padding around its child.
func PaddingV(child Element, p int) *padding {
	return &padding{
		child:  child,
		top:    p,
		bottom: p,
	}
}

func (p *padding) MinSize() (w, h int) {
	cw, ch := p.child.MinSize()
	return cw + p.left + p.right, ch + p.top + p.bottom
}

func (p *padding) Render(s Screen, x, y, width, height int, parent Style) {
	// Compute inner rectangle after padding
	innerX := x + p.left
	innerY := y + p.top
	innerW := width - p.left - p.right
	innerH := height - p.top - p.bottom

	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}

	p.child.Render(s, innerX, innerY, innerW, innerH, parent)
}

func (p *padding) Contains(px, py, x, y, w, h int) bool {
	innerX := x + p.left
	innerY := y + p.top
	innerW := w - p.left - p.right
	innerH := h - p.top - p.bottom
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}
	return p.child.Contains(px, py, innerX, innerY, innerW, innerH)
}

// ---------------------------------------------------------------------
// APP RUNNER (optional helper)
// ---------------------------------------------------------------------

type App struct {
	Root    Element
	Screen  Screen
	Focuser Focuser
}

func NewApp(root Element) *App {
	s, err := tcell.NewScreen()
	if err != nil {
		panic(err)
	}
	return &App{Root: root, Screen: s}
}

func (a *App) Run() error {
	if err := a.Screen.Init(); err != nil {
		return err
	}
	defer a.Screen.Fini()
	a.Screen.EnableMouse()

	draw := func() {
		a.Screen.Clear()
		w, h := a.Screen.Size()
		a.Root.Render(a.Screen, 0, 0, w, h, Style{})
		a.Screen.Show()
	}
	draw()

	for {
		ev := a.Screen.PollEvent()
		switch ev := ev.(type) {
		case *EventResize:
			a.Screen.Sync()
			draw()
		case *EventKey:
			switch ev.Key() {
			case tcell.KeyEscape:
				return nil
			}
		case *EventMouse:
			x, y := ev.Position()
			if ev.Buttons()&tcell.Button1 != 0 {
				a.handleClick(x, y)
				draw()
			}
		}
	}
}

func (a *App) Focus(f Focuser) {
	a.Focuser.Unfocus()
	f.Focus()
	a.Focuser = f
}

func (a *App) handleClick(px, py int) {
	w, h := a.Screen.Size()
	rw, rh := a.Root.MinSize()
	rx := (w - rw) / 2
	ry := (h - rh) / 2
	a.walk(a.Root, rx, ry, rw, rh, px, py)
}

func (a *App) walk(e Element, x, y, w, h, px, py int) {
	if btn, ok := e.(*button); ok && e.Contains(px, py, x, y, w, h) {
		if btn.onClick != nil {
			btn.onClick()
		}
	}
	if v, ok := e.(*vstack); ok {
		used := 0
		for _, child := range v.children {
			_, ch := child.MinSize()
			if used+ch > h {
				ch = h - used
			}
			if ch == 0 {
				break
			}
			a.walk(child, x, y+used, w, ch, px, py)
			used += ch
		}
	}
	if hs, ok := e.(*hstack); ok {
		used := 0
		for _, child := range hs.children {
			cw, _ := child.MinSize()
			if used+cw > w {
				cw = w - used
			}
			if cw == 0 {
				break
			}
			a.walk(child, x+used, y, cw, h, px, py)
			used += cw
		}
	}
}

func rectContains(px, py, x, y, w, h int) bool {
	return px >= x && px < x+w && py >= y && py < y+h
}
