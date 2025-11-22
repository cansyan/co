// Package ui provides a lightweight text user interface toolkit built on top of tcell.
// It offers a clean event–state–render pipeline with basic UI components and layouts.
package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

type Screen = tcell.Screen
type EventKey = tcell.EventKey
type EventMouse = tcell.EventMouse
type EventResize = tcell.EventResize
type Color = tcell.Color

// Element is the interface implemented by all UI elements.
type Element interface {
	MinSize() (w, h int)
	// Layout computes the layout node for this element given the position and size.
	// Elements inside the node will be rendered by drawTree()
	Layout(x, y, w, h int) *LayoutNode
	// Render draws the element onto the screen within the given rectangle and style.
	Render(s Screen, rect Rect, style Style)

	OnMouseEnter()
	OnMouseLeave()
	OnMouseDown(x, y int) // x, y is relative to element
	OnMouseUp(x, y int)   // x, y is relative to element
	OnMouseWheel(dy int)  // vertical scroll delta (dy > 0 means scroll up, dy < 0 means scroll down)
	OnFocus() Element     // returns the element that should receive focus, can be self or child
	OnBlur()
}

type LayoutNode struct {
	Element  Element
	Rect     Rect
	Children []*LayoutNode
}

type Rect struct {
	X, Y, W, H int
}

// BasicElement provides default no-op implementations for Element methods.
type BasicElement struct{}

func (b *BasicElement) MinSize() (int, int)                     { panic("not implemented") }
func (b *BasicElement) Layout(x, y, w, h int) *LayoutNode       { panic("not implemented") }
func (b *BasicElement) Render(s Screen, rect Rect, style Style) { panic("not implemented") }
func (b *BasicElement) OnMouseEnter()                           {}
func (b *BasicElement) OnMouseLeave()                           {}
func (b *BasicElement) OnMouseDown(x, y int)                    {}
func (b *BasicElement) OnMouseUp(x, y int)                      {}
func (b *BasicElement) OnMouseWheel(dy int)                     {}
func (b *BasicElement) OnFocus() Element                        { return b }
func (b *BasicElement) OnBlur()                                 {}

// KeyHandler is implemented by elements that can handle key events.
type KeyHandler interface {
	HandleKey(ev *tcell.EventKey)
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

type Text struct {
	BasicElement
	content string
	style   Style
}

func NewText(c string) *Text { return &Text{content: c, style: DefaultStyle} }

func (t *Text) SetText(c string) { t.content = c }

func (t *Text) Bold() *Text      { t.style.Bold = true; return t }
func (t *Text) Italic() *Text    { t.style.Italic = true; return t }
func (t *Text) Underline() *Text { t.style.Underline = true; return t }
func (t *Text) Foreground(c string) *Text {
	t.style.FG = tcell.ColorNames[c]
	return t
}
func (t *Text) Background(c string) *Text {
	t.style.BG = tcell.ColorNames[c]
	return t
}

func (t *Text) MinSize() (int, int) { return len(t.content), 1 }
func (t *Text) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (t *Text) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(t.style)
	for i, r := range t.content {
		if i >= rect.W {
			break
		}
		s.SetContent(rect.X+i, rect.Y, r, nil, st.Apply())
	}
}

type Button struct {
	BasicElement
	Label   string
	style   Style
	OnClick func()
	hovered bool
	pressed bool
}

// NewButton creates a new button element with the given label.
func NewButton(label string) *Button {
	return &Button{Label: label, style: DefaultStyle}
}
func (b *Button) Foreground(c string) *Button {
	b.style.FG = tcell.ColorNames[c]
	return b
}
func (b *Button) Background(c string) *Button {
	b.style.BG = tcell.ColorNames[c]
	return b
}

func (b *Button) MinSize() (int, int) { return runewidth.StringWidth(b.Label) + 2, 1 }
func (b *Button) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: b,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (b *Button) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(b.style).Apply()
	if b.hovered {
		st = st.Dim(false).Bold(true) // or underline
	}
	if b.pressed {
		st = st.Reverse(true)
	}
	label := " " + b.Label + " "
	for i, r := range label {
		if i >= rect.W {
			break
		}
		s.SetContent(rect.X+i, rect.Y, r, nil, st)
	}
}

func (b *Button) OnMouseEnter() { b.hovered = true }

func (b *Button) OnMouseLeave() {
	b.hovered = false
	b.pressed = false // cancel
}

func (b *Button) OnMouseDown(x, y int) {
	b.pressed = true
}

func (b *Button) OnMouseUp(x, y int) {
	if b.pressed && b.hovered {
		// real click
		if b.OnClick != nil {
			b.OnClick()
		}
	}
	b.pressed = false
}

type divider struct {
	BasicElement
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
	BasicElement
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

func (v *vstack) Append(e Element) *vstack { v.children = append(v.children, e); return v }

// Spacing sets the spacing (in rows) between child elements.
func (v *vstack) Spacing(p int) *vstack {
	v.spacing = p
	return v
}

type hstack struct {
	BasicElement
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

func (hs *hstack) Append(e Element) *hstack { hs.children = append(hs.children, e); return hs }

// Spacing sets the spacing (in columns) between child elements.
func (hs *hstack) Spacing(p int) *hstack { hs.spacing = p; return hs }

type fill struct {
	BasicElement
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
	BasicElement
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

type border struct {
	BasicElement
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

type empty struct {
	BasicElement
}

func (e empty) MinSize() (int, int)               { return 0, 0 }
func (e empty) Layout(x, y, w, h int) *LayoutNode { return nil }
func (e empty) Render(Screen, Rect, Style)        {}

func Spacer() *fill {
	return Fill(new(empty))
}

// ---------------------------------------------------------------------
// APP RUNNER (optional helper)
// ---------------------------------------------------------------------

type App struct {
	Root    Element
	Screen  Screen
	focused Element
	hovered Element
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

// find deepest node whose Rect contains (x, y)
func hitTest(node *LayoutNode, x, y int) *LayoutNode {
	if node == nil {
		return nil
	}
	r := node.Rect
	if x < r.X || y < r.Y || x >= r.X+r.W || y >= r.Y+r.H {
		return nil
	}

	// Search children first (to get the most specific element)
	for _, c := range node.Children {
		if n := hitTest(c, x, y); n != nil {
			return n
		}
	}
	return node
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

			if a.focused != nil {
				if h, ok := a.focused.(KeyHandler); ok {
					h.HandleKey(ev)
					// redrawing after every event is efficient enough
					// and the mose concise for simple TUI
					draw()
				}
			}
		case *EventMouse:
			// log.Print("mouse : ", ev.Buttons())
			x, y := ev.Position()
			node := hitTest(a.tree, x, y)
			if node == nil {
				continue
			}

			e := node.Element
			switch ev.Buttons() {
			case tcell.ButtonPrimary:
				e.OnMouseDown(x-node.Rect.X, y-node.Rect.Y)
				// focus/blur
				if e != a.focused {
					if a.focused != nil {
						a.focused.OnBlur()
					}
					a.focused = e.OnFocus()
				}
			case tcell.WheelUp:
				e.OnMouseWheel(-1)
			case tcell.WheelDown:
				e.OnMouseWheel(1)
			default:
				// hover enter/leave
				if e != a.hovered {
					if a.hovered != nil {
						a.hovered.OnMouseLeave()
					}
					e.OnMouseEnter()
					a.hovered = e
				}

				e.OnMouseUp(x-node.Rect.X, y-node.Rect.Y)
			}
			draw()
		}
	}
}

func (a *App) Stop() {
	close(a.done)
}

// TextField is a single-line editable text input field.
type TextField struct {
	BasicElement
	text     []rune
	cursor   int
	focused  bool
	style    Style
	onChange func(string)
}

func NewTextField() *TextField {
	return &TextField{
		style: DefaultStyle,
	}
}

func (t *TextField) Text() string {
	return string(t.text)
}

func (t *TextField) SetText(s string) {
	t.text = []rune(s)
	t.cursor = len(t.text)
}

func (t *TextField) OnChange(fn func(string)) *TextField {
	t.onChange = fn
	return t
}

func (t *TextField) Foreground(c string) *TextField {
	t.style.FG = tcell.ColorNames[c]
	return t
}
func (t *TextField) Background(c string) *TextField {
	t.style.BG = tcell.ColorNames[c]
	return t
}

func (t *TextField) MinSize() (int, int) { return 10, 1 }

func (t *TextField) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (t *TextField) Render(s Screen, rect Rect, style Style) {
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

func (t *TextField) OnFocus() Element { t.focused = true; return t }
func (t *TextField) OnBlur()          { t.focused = false }

func (t *TextField) HandleKey(ev *tcell.EventKey) {
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

func (t *TextField) OnMouseDown(x, y int) {
	if x < 0 {
		x = 0
	}
	if x > len(t.text) {
		x = len(t.text)
	}
	t.cursor = x
}

// TextEditor is a multi-line editable text area.
type TextEditor struct {
	BasicElement
	content  [][]rune // simple 2D slice of runes, avoid over-engineering
	row      int      // Current line index
	col      int      // Cursor column index (rune index)
	topRow   int      // Top visible line index (for vertical scrolling)
	focused  bool
	style    Style
	viewH    int // last rendered height
	onChange func()
}

func NewTextEditor() *TextEditor {
	return &TextEditor{
		content: [][]rune{{}}, // Start with one empty line of runes
		style:   DefaultStyle,
	}
}

func (t *TextEditor) Foreground(c string) *TextEditor {
	t.style.FG = tcell.ColorNames[c]
	return t
}
func (t *TextEditor) Background(c string) *TextEditor {
	t.style.BG = tcell.ColorNames[c]
	return t
}

func (t *TextEditor) String() string {
	var lines []string
	for _, line := range t.content {
		lines = append(lines, string(line))
	}
	return strings.Join(lines, "\n")
}

func (t *TextEditor) SetText(s string) {
	lines := strings.Split(s, "\n")
	t.content = make([][]rune, len(lines))
	for i, line := range lines {
		t.content[i] = []rune(line)
	}
	t.row = 0
	t.col = 0
	t.adjustCol()
}

func (t *TextEditor) adjustCol() {
	if t.row < len(t.content) {
		lineLen := len(t.content[t.row])
		if t.col > lineLen {
			t.col = lineLen
		}
	}
}

func (t *TextEditor) MinSize() (int, int) {
	// Fixed width: 5 columns for line numbers + 20 for content
	return 25, 5
}

func (t *TextEditor) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (t *TextEditor) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(t.style).Apply()
	t.viewH = rect.H

	// --- 1. Fixed offsets for Line Numbers ---
	lineNumWidth := 5

	// Dynamic width calculation (for proper right-justification)
	numLines := len(t.content)
	if numLines == 0 {
		numLines = 1
	}
	actualNumDigits := len(fmt.Sprintf("%d", numLines))
	if actualNumDigits > 4 {
		lineNumWidth = actualNumDigits + 1
	} else {
		lineNumWidth = 5
	}

	contentX := rect.X + lineNumWidth
	contentW := rect.W - lineNumWidth

	if contentW <= 0 {
		return
	}

	lineNumStyle := st.Reverse(false).Foreground(tcell.ColorSilver)

	var finalCursorX, finalCursorY int
	cursorFound := false
	// --- 2. Loop over visible rows ---
	for i := 0; i < rect.H; i++ {
		contentRow := i + t.topRow
		if contentRow >= len(t.content) {
			break
		}

		line := t.content[contentRow]
		isCursorLine := contentRow == t.row

		// A. Render Line Number (UNCONDITIONAL)
		lineNum := contentRow + 1
		numStr := fmt.Sprintf("%*d ", lineNumWidth-1, lineNum)

		for j, r := range numStr {
			s.SetContent(rect.X+j, rect.Y+i, r, nil, lineNumStyle)
		}

		// B. Render Line Content
		var screenCol int

		for j, r := range line {
			rw := runewidth.RuneWidth(r)

			// Check if we reached the cursor position
			if isCursorLine && j == t.col {
				finalCursorX = contentX + screenCol
				finalCursorY = rect.Y + i
				cursorFound = true
			}

			if screenCol+rw > contentW {
				break
			}

			s.SetContent(contentX+screenCol, rect.Y+i, r, nil, st)
			screenCol += rw
		}

		// C. Handle cursor placed at the very end of the line
		if isCursorLine && t.col == len(line) {
			finalCursorX = contentX + screenCol
			finalCursorY = rect.Y + i
			cursorFound = true
		}
	}

	// --- 3. Place the cursor ---
	if t.focused && cursorFound {
		s.ShowCursor(finalCursorX, finalCursorY)
	} else {
		s.HideCursor()
	}
}

func (t *TextEditor) OnFocus() Element { t.focused = true; return t }
func (t *TextEditor) OnBlur()          { t.focused = false }

func (t *TextEditor) HandleKey(ev *tcell.EventKey) {
	if !t.focused {
		return
	}

	currentLine := t.content[t.row]
	currentLineLen := len(currentLine)

	switch ev.Key() {
	case tcell.KeyUp:
		if t.row > 0 {
			t.row--
			t.adjustCol()
			t.adjustScroll()
		}
	case tcell.KeyDown:
		if t.row < len(t.content)-1 {
			t.row++
			t.adjustCol()
			t.adjustScroll()
		}
	case tcell.KeyLeft:
		if t.col > 0 {
			t.col--
		} else if t.row > 0 {
			t.row--
			t.col = len(t.content[t.row]) // End of previous line
			t.adjustScroll()
		}
	case tcell.KeyRight:
		if t.col < currentLineLen {
			t.col++
		} else if t.row < len(t.content)-1 {
			t.row++
			t.col = 0 // Start of next line
			t.adjustScroll()
		}
	case tcell.KeyEnter:
		head := currentLine[:t.col]
		tail := currentLine[t.col:]

		t.content[t.row] = head
		newLine := tail

		t.content = slices.Insert(t.content, t.row+1, newLine)

		t.row++
		t.col = 0
		t.adjustScroll()

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if t.col > 0 {
			t.content[t.row] = slices.Delete(currentLine, t.col-1, t.col)
			t.col--
		} else if t.row > 0 {
			prevLine := t.content[t.row-1]
			t.col = len(prevLine)
			t.content[t.row-1] = append(prevLine, currentLine...)

			t.content = slices.Delete(t.content, t.row, t.row+1)
			t.row--
			t.adjustScroll()
		}
	case tcell.KeyDelete:
		if t.col < currentLineLen {
			t.content[t.row] = slices.Delete(currentLine, t.col, t.col+1)
		} else if t.row < len(t.content)-1 {
			t.content[t.row] = append(currentLine, t.content[t.row+1]...)
			t.content = slices.Delete(t.content, t.row+1, t.row+2)
		}
	case tcell.KeyRune:
		r := ev.Rune()
		t.content[t.row] = slices.Insert(currentLine, t.col, r)
		t.col++
	}

	t.onChange()
}

func (t *TextEditor) OnMouseDown(x, y int) {
	// --- 1. Recalculate Content Area Offset (Must match Render() logic) ---
	lineNumWidth := 5
	numLines := len(t.content)
	if numLines == 0 {
		numLines = 1
	}
	actualNumDigits := len(fmt.Sprintf("%d", numLines))
	if actualNumDigits > 4 {
		lineNumWidth = actualNumDigits + 1
	} else {
		lineNumWidth = 5
	}

	// 2. Calculate the target row (relative to content)
	targetRow := y + t.topRow

	// Clamp the target row
	if targetRow < 0 {
		t.row = 0
	} else if targetRow >= len(t.content) {
		t.row = len(t.content) - 1
	} else {
		t.row = targetRow
	}

	if t.row < 0 {
		return
	}

	currentLine := t.content[t.row]

	clickedX := max(x-lineNumWidth, 0)

	// 3. Calculate the target column (rune index)
	targetCol := 0
	displayWidth := 0
	for i, r := range currentLine {
		rw := runewidth.RuneWidth(r)

		// Check if the clicked X is within the display width of the current rune.
		if displayWidth+rw/2 >= clickedX {
			targetCol = i
			break
		}

		displayWidth += rw

		if displayWidth >= clickedX {
			targetCol = i + 1
			break
		}
	}

	// 4. Handle a click past the end of the line
	if displayWidth < clickedX {
		targetCol = len(currentLine)
	}

	t.col = targetCol
	t.adjustCol()
	t.onChange()
}

func (t *TextEditor) OnMouseWheel(dy int) {
	if dy < 0 {
		// scroll down
		t.topRow = max(0, t.topRow+dy)
	} else if dy > 0 {
		// scroll up
		t.topRow = min(len(t.content)-1, t.topRow+dy)
	}
}

// adjustScroll ensures the cursor (t.row) is visible on the screen.
func (t *TextEditor) adjustScroll() {
	// Scroll down if cursor is below the visible area
	if t.row >= t.topRow+t.viewH {
		t.topRow = t.row - t.viewH + 1
	}
	// Scroll up if cursor is above the visible area
	if t.row < t.topRow {
		t.topRow = t.row
	}
}

// Cursor returns the current cursor position
func (t *TextEditor) Cursor() (row int, col int) {
	return t.row, t.col
}

// OnChange sets a callback function that is called whenever the text content changes.
func (t *TextEditor) OnChange(fn func()) {
	t.onChange = fn
}

type ListItem struct {
	Text    string
	OnClick func()
}

type List struct {
	BasicElement
	items       []ListItem
	selected    int // -1 means nothing selected, changes only on click
	style       Style
	hoverStyle  Style
	selectStyle Style // e.g. reversed background
	pressed     bool
}

func NewList() *List {
	hoverStyle := DefaultStyle
	hoverStyle.Bold = true
	selectStyle := DefaultStyle
	selectStyle.Reversed = true

	return &List{
		selected:    -1,
		style:       DefaultStyle,
		hoverStyle:  hoverStyle,
		selectStyle: selectStyle,
	}
}

func (l *List) Append(text string, onClick func()) {
	l.items = append(l.items, ListItem{Text: text, OnClick: onClick})
}

func (l *List) MinSize() (int, int) {
	maxW := 10
	for _, it := range l.items {
		if w := runewidth.StringWidth(it.Text); w > maxW {
			maxW = w
		}
	}
	return maxW + 2, len(l.items) // a bit of padding + one row per item
}

func (l *List) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: l,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (l *List) Render(s Screen, rect Rect, style Style) {
	base := style.Merge(l.style)

	for i, item := range l.items {
		if i >= rect.H {
			break
		}

		st := base
		if i == l.selected {
			st = st.Merge(l.selectStyle)
		}

		label := fmt.Sprintf(" %s ", item.Text)
		if w := runewidth.StringWidth(label); w > rect.W {
			label = runewidth.Truncate(label, rect.W, "…")
		}

		for col, r := range label {
			s.SetContent(rect.X+col, rect.Y+i, r, nil, st.Apply())
		}
	}
}

func (l *List) OnMouseDown(x, y int) {
	l.pressed = true
}

func (l *List) OnMouseUp(x, y int) {
	if !l.pressed {
		return
	}

	if y >= 0 && y < len(l.items) {
		l.selected = y
		if l.items[y].OnClick != nil {
			l.items[y].OnClick()
		}
	}
	l.pressed = false
}

type Tabs struct {
	BasicElement
	labels   []string
	items    []Element
	selected int
	pressed  bool
}

func (t *Tabs) Append(label string, content Element) *Tabs {
	t.labels = append(t.labels, label)
	t.items = append(t.items, content)
	return t
}

func (t *Tabs) MinSize() (int, int) {
	maxW, maxH := 0, 0
	for _, item := range t.items {
		w, h := item.MinSize()
		if w > maxW {
			maxW = w
		}
		if h > maxH {
			maxH = h
		}
	}
	return maxW, maxH + 1 // +1 for tab labels
}

func (t *Tabs) Layout(x, y, w, h int) *LayoutNode {
	n := &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}

	// Layout the selected tab content
	if t.selected >= 0 && t.selected < len(t.items) {
		childNode := t.items[t.selected].Layout(x, y+1, w, h-1)
		n.Children = append(n.Children, childNode)
	}
	return n
}

func (t *Tabs) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(DefaultStyle).Apply()

	// Render tab labels
	currentX := rect.X
	for i, label := range t.labels {
		tabLabel := " " + label + " "
		tabW := runewidth.StringWidth(tabLabel)
		if currentX+tabW > rect.X+rect.W {
			break // no more space
		}

		tabStyle := st
		if i == t.selected {
			tabStyle = tabStyle.Reverse(true)
		}

		for col, r := range tabLabel {
			s.SetContent(currentX+col, rect.Y, r, nil, tabStyle)
		}
		currentX += tabW
	}

	// tab content rendered by drawTree()

	if t.selected >= 0 && t.selected < len(t.items) {
		switch t.items[t.selected].(type) {
		case *TextEditor, *TextField:
		default:
			s.HideCursor()
		}
	}
}

func (t *Tabs) OnMouseDown(x, y int) {
	t.pressed = true
	if y == 0 {
		// Click on tab labels
		currentX := 0
		for i, label := range t.labels {
			tabLabel := " " + label + " "
			tabW := runewidth.StringWidth(tabLabel)
			if x >= currentX && x < currentX+tabW {
				t.selected = i
				t.items[i].OnFocus()
			} else {
				t.items[i].OnBlur()
			}
			currentX += tabW
		}
	}
}

func (t *Tabs) OnMouseUp(x, y int) {
	t.pressed = false
}

func (t *Tabs) OnFocus() Element {
	return t.items[t.selected].OnFocus()
}

func (t *Tabs) OnBlur() {
	t.items[t.selected].OnBlur()
}
