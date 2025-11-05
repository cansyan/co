// Package ui provides a simple terminal UI toolkit built on top of tcell.
package ui

import (
	"github.com/gdamore/tcell/v2"
)

type Screen = tcell.Screen
type EventKey = tcell.EventKey
type EventMouse = tcell.EventMouse
type EventResize = tcell.EventResize
type Color = tcell.Color

type Element interface {
	Render(s Screen, x, y, width, height int, parent Style)
	MinSize() (w, h int)
	// HitTest find the deepest element at the given point (px, py)
	HitTest(px, py, x, y, w, h int) Element
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
func (t *text) HitTest(px, py, x, y, w, h int) Element {
	b := rectContains(px, py, x, y, w, h)
	if !b {
		return nil
	}
	return t
}

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
	for i, r := range label {
		if x+i >= x+width {
			break
		}
		s.SetContent(x+i, y, r, nil, st.Apply())
	}
}
func (b *button) HitTest(px, py, x, y, w, h int) Element {
	contains := rectContains(px, py, x, y, w, h)
	if !contains {
		return nil
	}
	return b
}

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
		}
	} else {
		for i := range height {
			s.SetContent(x+width-1, y+i, '|', nil, st.Apply())
		}
	}
}
func (d *divider) HitTest(px, py, x, y, w, h int) Element { return nil }

// ---------------------------------------------------------------------
// Layouts
// ---------------------------------------------------------------------

type vstack struct {
	children []Element
	style    Style
	spacing  int
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
	for i, child := range v.children {
		cw, ch := child.MinSize()
		if cw > maxW {
			maxW = cw
		}
		totalH += ch
		if i < len(v.children)-1 {
			totalH += v.spacing
		}
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
	for i, child := range v.children {
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
		if i < len(v.children)-1 {
			used += v.spacing
		}
	}
}

func (v *vstack) HitTest(px, py, x, y, w, h int) Element {
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
	extra := max(h-totalMinHeight, 0)

	// Second pass: render children with adjusted heights
	used := 0
	for i, child := range v.children {
		if div, ok := child.(*divider); ok {
			div.vertical = false
		}
		_, ch := child.MinSize()
		if _, ok := child.(*grow); ok && growCount > 0 {
			ch += extra / growCount
		}
		if used+ch > h {
			ch = h - used
		}
		if ch > 0 {
			if a := child.HitTest(px, py, x, y+used, w, ch); a != nil {
				return a
			}
		}
		used += ch
		if i < len(v.children)-1 {
			used += v.spacing
		}
	}
	return nil
}

func (v *vstack) Add(e Element) *vstack { v.children = append(v.children, e); return v }

// Spacing sets the spacing (in rows) between child elements.
func (v *vstack) Spacing(p int) *vstack {
	v.spacing = p
	return v
}

type hstack struct {
	children []Element
	style    Style
	spacing  int
}

func HStack(children ...Element) *hstack {
	return &hstack{children: children, style: DefaultStyle}
}
func (h *hstack) MinSize() (int, int) {
	totalW, maxH := 0, 0
	for i, child := range h.children {
		cw, ch := child.MinSize()
		totalW += cw
		if ch > maxH {
			maxH = ch
		}
		if i < len(h.children)-1 {
			totalW += h.spacing
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
	for i, child := range h.children {
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
		if i < len(h.children)-1 {
			used += h.spacing
		}
	}
}
func (h *hstack) HitTest(px, py, x, y, w, height int) Element {
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
	extra := max(w-totalMinWidth, 0)

	// Second pass: layout children
	used := 0
	for i, child := range h.children {
		if div, ok := child.(*divider); ok {
			div.vertical = true
		}
		cw, _ := child.MinSize()
		if _, ok := child.(*grow); ok && growCount > 0 {
			cw += extra / growCount
		}
		if used+cw > w {
			cw = w - used
		}
		if cw > 0 {
			if a := child.HitTest(px, py, x+used, y, cw, height); a != nil {
				return a
			}
		}
		used += cw
		if i < len(h.children)-1 {
			used += h.spacing
		}
	}
	return nil
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

// Spacing sets the spacing (in columns) between child elements.
func (h *hstack) Spacing(p int) *hstack { h.spacing = p; return h }

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

func (g *grow) HitTest(px, py, x, y, w, h int) Element {
	return g.child.HitTest(px, py, x, y, w, h)
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

func (p *padding) Contains(px, py, x, y, w, h int) Element {
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
	return p.child.HitTest(px, py, innerX, innerY, innerW, innerH)
}

type Empty struct{}

func (e Empty) MinSize() (int, int)                          { return 0, 0 }
func (e Empty) Render(Screen, int, int, int, int, Style)     {}
func (e Empty) HitTest(int, int, int, int, int, int) Element { return nil }

func Spacer() *grow {
	return Grow(Empty{})
}

// ---------------------------------------------------------------------
// APP RUNNER (optional helper)
// ---------------------------------------------------------------------

type App struct {
	Root    Element
	Screen  Screen
	Focuser Focuser
	hover   Element
	done    chan struct{}
}

func NewApp(root Element) *App {
	s, err := tcell.NewScreen()
	if err != nil {
		panic(err)
	}
	return &App{Root: root, Screen: s, done: make(chan struct{})}
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

	// Keep it simple, draws everything from scratch each frame.
	// One day might evolve toward a retained mode,
	// keeping track of layout tree, but doesn't need it yet.
	for {
		select {
		case <-a.done:
			return nil
		default:
		}

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
			w, h := a.Screen.Size()
			i := a.Root.HitTest(x, y, 0, 0, w, h)
			if i == nil {
				continue
			}

			// hover
			if i != a.hover {
				if prevBtn, ok := a.hover.(*button); ok {
					prevBtn.style.Reversed = !prevBtn.style.Reversed
				}
				if btn, ok := i.(*button); ok {
					btn.style.Reversed = !btn.style.Reversed
				}
				a.hover = i
				draw()
			}

			// click
			if ev.Buttons()&tcell.Button1 != 0 {
				if btn, ok := i.(*button); ok && btn.onClick != nil {
					btn.onClick()
					draw()
				}
			}
		}
	}
}

func (a *App) Focus(f Focuser) {
	a.Focuser.Unfocus()
	f.Focus()
	a.Focuser = f
}

func (a *App) Stop() {
	close(a.done)
}

func rectContains(px, py, x, y, w, h int) bool {
	return px >= x && px < x+w && py >= y && py < y+h
}
