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

type LayoutNode struct {
	Element  Element
	Rect     Rect
	Children []*LayoutNode
}

type Rect struct {
	X, Y, W, H int
}

type Element interface {
	MinSize() (w, h int)
	// Layout computes the layout node for this element given the position and size.
	Layout(x, y, w, h int) *LayoutNode
	// Render draws the element onto the screen within the given rectangle and style.
	Render(s Screen, rect Rect, style Style)
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
func (t *text) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (t *text) Render(s Screen, rect Rect, style Style) {
	st := mergeStyle(style, t.style)
	for i, r := range t.content {
		if i >= rect.W {
			break
		}
		s.SetContent(rect.X+i, rect.Y, r, nil, st.Apply())
	}
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
func (b *button) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: b,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (b *button) Render(s Screen, rect Rect, parent Style) {
	st := mergeStyle(parent, b.style)
	label := " " + b.label + " "
	for i, r := range label {
		if i >= rect.W {
			break
		}
		s.SetContent(rect.X+i, rect.Y, r, nil, st.Apply())
	}
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
func (d *divider) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: d,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (d *divider) Render(s Screen, rect Rect, parent Style) {
	st := mergeStyle(parent, d.style)
	if !d.vertical {
		for i := range rect.W {
			s.SetContent(rect.X+i, rect.Y+rect.H-1, '-', nil, st.Apply())
		}
	} else {
		for i := range rect.H {
			s.SetContent(rect.X+rect.W-1, rect.Y+i, '|', nil, st.Apply())
		}
	}
}

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

func (v *vstack) Layout(x, y, w, h int) *LayoutNode {
	n := &LayoutNode{
		Element: v,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}

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
			childNode := child.Layout(x, y+used, w, ch)
			n.Children = append(n.Children, childNode)
		}
		used += ch
		if i < len(v.children)-1 {
			used += v.spacing
		}
	}
	return n
}

func (v *vstack) Render(s Screen, rect Rect, parent Style) {
	// do nothing, children are rendered in drawTree()
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
func (hs *hstack) MinSize() (int, int) {
	totalW, maxH := 0, 0
	for i, child := range hs.children {
		cw, ch := child.MinSize()
		totalW += cw
		if ch > maxH {
			maxH = ch
		}
		if i < len(hs.children)-1 {
			totalW += hs.spacing
		}
	}
	return totalW, maxH
}

func (hs *hstack) Layout(x, y, w, h int) *LayoutNode {
	n := &LayoutNode{
		Element: hs,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
	// First pass: measure children
	totalMinWidth := 0
	growCount := 0
	for _, child := range hs.children {
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
	for i, child := range hs.children {
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
			childNode := child.Layout(x+used, y, cw, h)
			n.Children = append(n.Children, childNode)
		}
		used += cw
		if i < len(hs.children)-1 {
			used += hs.spacing
		}
	}
	return n
}

func (hs *hstack) Render(s Screen, rect Rect, parent Style) {
	// do nothing, children are rendered in drawTree()
}

func (hs *hstack) Foreground(color string) *hstack {
	hs.style.FG = tcell.ColorNames[color]
	return hs
}
func (hs *hstack) Background(color string) *hstack {
	hs.style.BG = tcell.ColorNames[color]
	return hs
}

func (hs *hstack) Add(e Element) *hstack { hs.children = append(hs.children, e); return hs }

// Spacing sets the spacing (in columns) between child elements.
func (hs *hstack) Spacing(p int) *hstack { hs.spacing = p; return hs }

type grow struct {
	child Element
}

// Grow creates a layout element that expands to fill available space.
// Should be used inside HStack or VStack.
func Grow(child Element) *grow {
	return &grow{child: child}
}

func (g *grow) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: g,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
		Children: []*LayoutNode{
			g.child.Layout(x, y, w, h),
		},
	}
}

func (g *grow) MinSize() (int, int) {
	return g.child.MinSize()
}

func (g *grow) Render(s Screen, rect Rect, parent Style) {
	// do nothing, child is rendered in drawTree()
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
func (p *padding) Layout(x, y, w, h int) *LayoutNode {
	// Compute inner rectangle after padding
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

	return &LayoutNode{
		Element: p,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
		Children: []*LayoutNode{
			p.child.Layout(innerX, innerY, innerW, innerH),
		},
	}
}

func (p *padding) Render(s Screen, rect Rect, parent Style) {
	// do nothing, child is rendered in drawTree()
}

type Empty struct{}

func (e Empty) MinSize() (int, int)               { return 0, 0 }
func (e Empty) Layout(x, y, w, h int) *LayoutNode { return nil }
func (e Empty) Render(Screen, Rect, Style)        {}

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
	Tree    *LayoutNode // layout tree
}

func NewApp(root Element) *App {
	s, err := tcell.NewScreen()
	if err != nil {
		panic(err)
	}
	return &App{Root: root, Screen: s, done: make(chan struct{})}
}

func drawTree(node *LayoutNode, s Screen, style Style) {
	if node == nil {
		return
	}

	node.Element.Render(s, node.Rect, style)
	for _, child := range node.Children {
		drawTree(child, s, style)
	}
}

// Render build layout tree and render
func (a *App) Render() {
	w, h := a.Screen.Size()
	a.Tree = a.Root.Layout(0, 0, w, h)
	drawTree(a.Tree, a.Screen, Style{})
}

func (a *App) HitTest(px, py int) Element {
	return walk(a.Tree, px, py)
}

func walk(node *LayoutNode, px, py int) Element {
	if node == nil {
		return nil
	}
	r := node.Rect
	if px < r.X || py < r.Y || px >= r.X+r.W || py >= r.Y+r.H {
		return nil
	}

	// Search children first (topmost)
	for _, c := range node.Children {
		if e := walk(c, px, py); e != nil {
			return e
		}
	}
	return node.Element
}
func (a *App) Run() error {
	if err := a.Screen.Init(); err != nil {
		return err
	}
	defer a.Screen.Fini()
	a.Screen.EnableMouse()

	draw := func() {
		a.Screen.Clear()
		a.Render()
		a.Screen.Show()
	}
	draw()

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
			e := a.HitTest(x, y)
			if e == nil {
				continue
			}
			// hover
			if e != a.hover {
				if prevBtn, ok := a.hover.(*button); ok {
					prevBtn.style.Reversed = !prevBtn.style.Reversed
				}
				if btn, ok := e.(*button); ok {
					btn.style.Reversed = !btn.style.Reversed
				}
				a.hover = e
				draw()
			}
			// click
			if ev.Buttons()&tcell.Button1 != 0 {
				if btn, ok := e.(*button); ok && btn.onClick != nil {
					btn.onClick()
					draw()
				}
				continue
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
