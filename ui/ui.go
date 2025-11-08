// Package ui provides a simple text user interface toolkit built on top of tcell.
package ui

import (
	"slices"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
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

// Element is the interface implemented by all UI elements.
type Element interface {
	MinSize() (w, h int)
	// Layout computes the layout node for this element given the position and size.
	Layout(x, y, w, h int) *LayoutNode
	// Render draws the element onto the screen within the given rectangle and style.
	Render(s Screen, rect Rect, style Style)
}

// Focusable is implemented by elements that can receive focus.
type Focusable interface {
	Focus()
	Unfocus()
	IsFocused() bool
}

// KeyHandler is implemented by elements that can handle key events.
type KeyHandler interface {
	HandleKey(ev *tcell.EventKey)
}

// MouseHandler is implemented by elements that can handle mouse events.
type MouseHandler interface {
	HandleMouse(ev *tcell.EventMouse, rect Rect)
}

type Style struct {
	FG        Color
	BG        Color
	Reversed  bool
	Bold      bool
	Italic    bool
	Underline bool
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
	if s.Underline {
		st = st.Underline(true)
	}
	if s.Reversed {
		st = st.Reverse(true)
	}
	return st
}

// Merge returns a new Style by applying the child style's non-default attributes
// over the receiver (parent) style.
func (s Style) Merge(child Style) Style {
	if child.FG == tcell.ColorDefault {
		child.FG = s.FG
	}
	if child.BG == tcell.ColorDefault {
		child.BG = s.BG
	}
	child.Bold = child.Bold || s.Bold
	child.Italic = child.Italic || s.Italic
	child.Reversed = child.Reversed || s.Reversed
	return child
}

// ---------------------------------------------------------------------
// components
// ---------------------------------------------------------------------

type text struct {
	content string
	style   Style
}

func Text(c string) *text { return &text{content: c, style: DefaultStyle} }

func (t *text) Bold() *text      { t.style.Bold = true; return t }
func (t *text) Italic() *text    { t.style.Italic = true; return t }
func (t *text) Underline() *text { t.style.Underline = true; return t }
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
	st := style.Merge(t.style)
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
func (b *button) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(b.style)
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
func (d *divider) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(d.style)
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

// VStack creates a vertical stack layout element.
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
	fillCount := 0
	for _, child := range v.children {
		_, ch := child.MinSize()
		totalMinHeight += ch
		if _, ok := child.(*fill); ok {
			fillCount++
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
		if _, ok := child.(*fill); ok && fillCount > 0 {
			ch += extra / fillCount
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

func (v *vstack) Render(s Screen, rect Rect, style Style) {
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

// HStack creates a horizontal stack layout element.
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
	fillCount := 0
	for _, child := range hs.children {
		cw, _ := child.MinSize()
		totalMinWidth += cw
		if _, ok := child.(*fill); ok {
			fillCount++
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
		if _, ok := child.(*fill); ok && fillCount > 0 {
			cw += extra / fillCount
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

func (hs *hstack) Render(s Screen, rect Rect, style Style) {
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

type fill struct {
	child Element
}

// Fill expands its child to fill available space.
// Should be used inside HStack or VStack.
func Fill(child Element) *fill {
	return &fill{child: child}
}

func (f *fill) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: f,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
		Children: []*LayoutNode{
			f.child.Layout(x, y, w, h),
		},
	}
}

func (f *fill) MinSize() (int, int) {
	return f.child.MinSize()
}

func (f *fill) Render(s Screen, rect Rect, style Style) {
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

func (p *padding) Render(s Screen, rect Rect, style Style) {
	// do nothing, child is rendered in drawTree()
}

type empty struct{}

func (e empty) MinSize() (int, int)               { return 0, 0 }
func (e empty) Layout(x, y, w, h int) *LayoutNode { return nil }
func (e empty) Render(Screen, Rect, Style)        {}

func Spacer() *fill {
	return Fill(empty{})
}

// ---------------------------------------------------------------------
// APP RUNNER (optional helper)
// ---------------------------------------------------------------------

type App struct {
	Root    Element
	Screen  Screen
	Focuser Focusable // currently focused element
	hover   Element
	done    chan struct{}
	tree    *LayoutNode // layout tree
	QuitKey tcell.Key   // key to quit the app, default is Escape
}

func NewApp(root Element) *App {
	s, err := tcell.NewScreen()
	if err != nil {
		panic(err)
	}
	return &App{
		Root:    root,
		Screen:  s,
		done:    make(chan struct{}),
		QuitKey: tcell.KeyEscape,
	}
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
	a.tree = a.Root.Layout(0, 0, w, h)
	drawTree(a.tree, a.Screen, DefaultStyle)
}

// hitTest returns the topmost Element at the given screen coordinates.
func (a *App) hitTest(px, py int) Element {
	var walk func(node *LayoutNode, px, py int) Element
	walk = func(node *LayoutNode, px, py int) Element {
		if node == nil {
			return nil
		}
		r := node.Rect
		if px < r.X || py < r.Y || px >= r.X+r.W || py >= r.Y+r.H {
			return nil
		}

		// Search children first (to get the most specific element)
		for _, c := range node.Children {
			if e := walk(c, px, py); e != nil {
				return e
			}
		}
		return node.Element
	}
	return walk(a.tree, px, py)
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
			if ev.Key() == a.QuitKey {
				return nil
			}

			if a.Focuser != nil {
				if h, ok := a.Focuser.(KeyHandler); ok {
					h.HandleKey(ev)
					// redrawing after every event is efficient enough
					// and the mose concise for simple TUI
					draw()
				}
			}
		case *EventMouse:
			x, y := ev.Position()
			e := a.hitTest(x, y)
			if e == nil {
				continue
			}

			// hover highlight
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

			// click, drag, scroll
			switch ev.Buttons() {
			case tcell.Button1:
				// left click
				if btn, ok := e.(*button); ok && btn.onClick != nil {
					btn.onClick()
				}
				if f, ok := e.(Focusable); ok {
					a.Focus(f)
				}
				// locate cursor
				if h, ok := e.(MouseHandler); ok {
					h.HandleMouse(ev, a.findRect(e))
				}
				draw()
			case tcell.WheelUp, tcell.WheelDown:
				if h, ok := e.(MouseHandler); ok {
					h.HandleMouse(ev, a.findRect(e))
				}
				draw()
			default:
				continue
			}
		}
	}
}

func (a *App) Focus(f Focusable) {
	if a.Focuser != nil {
		a.Focuser.Unfocus()
	}
	a.Focuser = f
	f.Focus()
}

func (a *App) findRect(e Element) Rect {
	var search func(*LayoutNode) *LayoutNode
	search = func(n *LayoutNode) *LayoutNode {
		if n == nil {
			return nil
		}
		if n.Element == e {
			return n
		}
		for _, c := range n.Children {
			if found := search(c); found != nil {
				return found
			}
		}
		return nil
	}
	if node := search(a.tree); node != nil {
		return node.Rect
	}
	return Rect{}
}

func (a *App) Stop() {
	close(a.done)
}

type textField struct {
	text     []rune
	cursor   int
	focused  bool
	style    Style
	onChange func(string)
}

// TextField creates a single-line editable text input.
func TextField() *textField {
	return &textField{
		style: DefaultStyle,
	}
}

func (t *textField) Text() string {
	return string(t.text)
}

func (t *textField) SetText(s string) {
	t.text = []rune(s)
	t.cursor = len(t.text)
}

func (t *textField) OnChange(fn func(string)) *textField {
	t.onChange = fn
	return t
}

func (t *textField) Foreground(c string) *textField {
	t.style.FG = tcell.ColorNames[c]
	return t
}
func (t *textField) Background(c string) *textField {
	t.style.BG = tcell.ColorNames[c]
	return t
}

func (t *textField) MinSize() (int, int) { return 10, 1 }

func (t *textField) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (t *textField) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(t.style).Apply()
	var totalW int
	for _, r := range t.text {
		rw := runewidth.RuneWidth(r)
		if totalW+rw > rect.W {
			break
		}
		s.SetContent(rect.X+totalW, rect.Y, r, nil, st)
		totalW += rw
	}
	if t.focused && t.cursor < rect.W {
		s.ShowCursor(rect.X+t.cursor, rect.Y)
	} else {
		s.HideCursor()
	}
}

// Focus implement Focuser
func (t *textField) Focus()   { t.focused = true }
func (t *textField) Unfocus() { t.focused = false }
func (t *textField) IsFocused() bool {
	return t.focused
}

func (t *textField) HandleKey(ev *tcell.EventKey) {
	if !t.focused {
		return
	}
	switch ev.Key() {
	case tcell.KeyLeft:
		if t.cursor > 0 {
			t.cursor--
		}
	case tcell.KeyRight:
		if t.cursor < len(t.text) {
			t.cursor++
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if t.cursor > 0 {
			t.text = slices.Delete(t.text, t.cursor-1, t.cursor)
			t.cursor--
			if t.onChange != nil {
				t.onChange(string(t.text))
			}
		}
	case tcell.KeyDelete:
		if t.cursor < len(t.text) {
			t.text = slices.Delete(t.text, t.cursor, t.cursor+1)
			if t.onChange != nil {
				t.onChange(string(t.text))
			}
		}
	case tcell.KeyRune:
		r := ev.Rune()
		t.text = slices.Insert(t.text, t.cursor, r)
		t.cursor++
		if t.onChange != nil {
			t.onChange(string(t.text))
		}
	}
}

func (t *textField) HandleMouse(ev *tcell.EventMouse, rect Rect) {
	x, _ := ev.Position()
	// Clamp to available width
	pos := x - rect.X
	if pos < 0 {
		pos = 0
	}
	if pos > len(t.text) {
		pos = len(t.text)
	}
	t.cursor = pos
}

type border struct {
	child Element
	style Style
}

// Border creates a layout element that draws a border around its child.
func Border(child Element) *border {
	return &border{child: child, style: DefaultStyle}
}

func (b *border) MinSize() (w, h int) {
	cw, ch := b.child.MinSize()
	return cw + 2, ch + 2
}
func (b *border) Layout(x, y, w, h int) *LayoutNode {
	// Compute inner rectangle after border
	innerX := x + 1
	innerY := y + 1
	innerW := w - 2
	innerH := h - 2
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}

	return &LayoutNode{
		Element: b,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
		Children: []*LayoutNode{
			b.child.Layout(innerX, innerY, innerW, innerH),
		},
	}
}
func (b *border) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(b.style).Apply()
	// Top and bottom borders
	for i := 0; i < rect.W; i++ {
		s.SetContent(rect.X+i, rect.Y, '-', nil, st)
		s.SetContent(rect.X+i, rect.Y+rect.H-1, '-', nil, st)
	}
	// Left and right borders
	for i := 0; i < rect.H; i++ {
		s.SetContent(rect.X, rect.Y+i, '|', nil, st)
		s.SetContent(rect.X+rect.W-1, rect.Y+i, '|', nil, st)
	}
	// Corners
	s.SetContent(rect.X, rect.Y, '+', nil, st)
	s.SetContent(rect.X+rect.W-1, rect.Y, '+', nil, st)
	s.SetContent(rect.X, rect.Y+rect.H-1, '+', nil, st)
	s.SetContent(rect.X+rect.W-1, rect.Y+rect.H-1, '+', nil, st)
}
