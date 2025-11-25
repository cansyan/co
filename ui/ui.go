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
}

// Hoverable represents an element that can respond to mouse hover.
// localX and localY indicate the mouse position relative to the element's top-left corner.
type Hoverable interface {
	OnMouseEnter()
	OnMouseLeave()
	OnMouseMove(localX, localY int)
}

// Clickable represents an element that can respond to mouse click actions.
// localX and localY indicate the mouse position relative to the element's top-left corner.
type Clickable interface {
	// OnMouseDown is called when the mouse button is pressed.
	OnMouseDown(localX, localY int)
	// OnMouseUp is called when the mouse button is released.
	OnMouseUp(localX, localY int)
}

// Scrollable represents an element that can respond to vertical scroll events.
type Scrollable interface {
	// OnScroll is called when a scroll action occurs.
	// dy is the scroll delta: dy > 0 means scrolling up, dy < 0 means scrolling down.
	OnScroll(dy int)
}

// Focusable represents an element that can receive focus.
type Focusable interface {
	// FocusTarget determines which element should actually receive focus.
	//   - To retain focus on the element itself, return the element (self).
	//   - To delegate focus to a child element, return that child.
	FocusTarget() Element
	// OnFocus is called when the element receives focus.
	OnFocus()
	// OnBlur is called when the element loses focus.
	OnBlur()
}

// KeyHandler is the interface that focusable elements can implement to handle key events.
type KeyHandler interface {
	HandleKey(ev *tcell.EventKey)
}

type LayoutNode struct {
	Element  Element
	Rect     Rect
	Children []*LayoutNode
}

type Rect struct {
	X, Y, W, H int
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

// helper function, render unicode properly.
func renderString(s Screen, x, y, w int, str string, style tcell.Style) {
	offset := 0
	for _, r := range str {
		if offset >= w {
			break
		}
		s.SetContent(x+offset, y, r, nil, style)
		offset += runewidth.RuneWidth(r)
	}
}

// ---------------------------------------------------------------------
// components
// ---------------------------------------------------------------------

type Text struct {
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
	st := style.Merge(t.style).Apply()
	renderString(s, rect.X, rect.Y, rect.W, t.content, st)
}

type Button struct {
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
	renderString(s, rect.X, rect.Y, rect.W, label, st)
}

func (b *Button) OnMouseEnter() { b.hovered = true }

func (b *Button) OnMouseLeave() {
	b.hovered = false
	b.pressed = false // cancel
}

func (b *Button) OnMouseMove(rx, ry int) {}

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

func (v *vstack) Append(e Element) *vstack { v.children = append(v.children, e); return v }

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

func (hs *hstack) Append(e Element) *hstack { hs.children = append(hs.children, e); return hs }

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

type empty struct{}

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
	Screen  Screen
	Root    Element
	focused Element
	hover   Element
	tree    *LayoutNode // layout tree
	done    chan struct{}
	QuitKey tcell.Key // key to quit the app, default is Escape

	clickX, clickY int
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

// find deepest matched element, return the coordinates relative to it
func hitTest(node *LayoutNode, x, y int) (Element, int, int) {
	if node == nil {
		return nil, 0, 0
	}
	r := node.Rect
	if x < r.X || y < r.Y || x >= r.X+r.W || y >= r.Y+r.H {
		return nil, 0, 0
	}

	// search the deepest child
	for _, child := range node.Children {
		if e, ox, oy := hitTest(child, x, y); e != nil {
			return e, ox, oy
		}
	}

	localX := x - node.Rect.X
	localY := y - node.Rect.Y
	return node.Element, localX, localY
}

func (a *App) Focus(e Element) {
	if e == nil {
		return
	}

	if a.focused != nil {
		if f, ok := a.focused.(Focusable); ok {
			f.OnBlur()
		}
	}
	a.focused = a.resolveFocus(e)
	if f, ok := a.focused.(Focusable); ok {
		f.OnFocus()
	}
}

func (a *App) resolveFocus(e Element) Element {
	for {
		f, ok := e.(Focusable)
		if !ok {
			return e
		}
		t := f.FocusTarget()
		if t == e {
			return e
		}
		e = t
	}
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
		case *EventKey:
			a.handleKey(ev)
		case *EventMouse:
			a.handleMouse(ev)
		}
		// redrawing after every event, keep simple and concise
		draw()
	}
}

func (a *App) handleKey(ev *tcell.EventKey) {
	if ev.Key() == a.QuitKey {
		close(a.done)
		return
	}

	if a.focused == nil {
		return
	}
	if h, ok := a.focused.(KeyHandler); ok {
		h.HandleKey(ev)
	}
}

// hover -> mouse down -> focus -> mouse up -> scroll
func (a *App) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	e, lx, ly := hitTest(a.tree, x, y)
	if e == nil {
		return
	}

	a.updateHover(e, lx, ly)

	switch ev.Buttons() {
	case tcell.ButtonPrimary:
		a.clickX, a.clickY = x, y
		// mouse down
		if i, ok := e.(Clickable); ok {
			i.OnMouseDown(lx, ly)
		}

		// shift focus
		a.Focus(e)
		if _, ok := a.focused.(KeyHandler); !ok {
			a.Screen.HideCursor()
		}
	case tcell.WheelUp:
		if i, ok := e.(Scrollable); ok {
			i.OnScroll(-1)
		}
	case tcell.WheelDown:
		if i, ok := e.(Scrollable); ok {
			i.OnScroll(1)
		}
	default:
		// mouse up
		if x == a.clickX && y == a.clickY {
			if i, ok := e.(Clickable); ok {
				i.OnMouseUp(lx, ly)
			}
		}
	}
}

func (a *App) updateHover(e Element, ox, oy int) {
	if a.hover != e {
		if h, ok := a.hover.(Hoverable); ok {
			h.OnMouseLeave()
		}
		if h, ok := e.(Hoverable); ok {
			h.OnMouseEnter()
		}
		a.hover = e
	}

	if h, ok := e.(Hoverable); ok {
		h.OnMouseMove(ox, oy)
	}
}

func (a *App) Stop() {
	close(a.done)
}

// TextField is a single-line editable text input field.
type TextField struct {
	text     []rune
	cursor   int
	active   bool
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
	if t.active && t.cursor < rect.W {
		s.ShowCursor(rect.X+t.cursor, rect.Y)
	} else {
		s.HideCursor()
	}
}

func (t *TextField) FocusTarget() Element { return t }
func (t *TextField) OnFocus()             { t.active = true }
func (t *TextField) OnBlur()              { t.active = false }

func (t *TextField) HandleKey(ev *tcell.EventKey) {
	if !t.active {
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
	content  [][]rune // simple 2D slice of runes, avoid over-engineering
	row      int      // Current line index
	col      int      // Cursor column index (rune index)
	offsetY  int      // Vertical scroll offset
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
		contentRow := i + t.offsetY
		if contentRow >= len(t.content) {
			break
		}

		line := t.content[contentRow]
		isCursorLine := contentRow == t.row

		// A. Render Line Number (UNCONDITIONAL)
		lineNum := contentRow + 1
		numStr := fmt.Sprintf("%*d ", lineNumWidth-1, lineNum)
		renderString(s, rect.X, rect.Y+i, lineNumWidth, numStr, lineNumStyle)

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
func (t *TextEditor) FocusTarget() Element { return t }
func (t *TextEditor) OnFocus()             { t.focused = true }
func (t *TextEditor) OnBlur()              { t.focused = false }

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
func (t *TextEditor) OnMouseUp(x, y int) {}
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
	targetRow := y + t.offsetY

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

func (t *TextEditor) OnScroll(dy int) {
	if dy < 0 {
		// scroll down
		t.offsetY = max(0, t.offsetY+dy)
	} else if dy > 0 {
		// scroll up
		t.offsetY = min(len(t.content)-1, t.offsetY+dy)
	}
}

// adjustScroll ensures the cursor (t.row) is visible on the screen.
func (t *TextEditor) adjustScroll() {
	// Scroll down if cursor is below the visible area
	if t.row >= t.offsetY+t.viewH {
		t.offsetY = t.row - t.viewH + 1
	}
	// Scroll up if cursor is above the visible area
	if t.row < t.offsetY {
		t.offsetY = t.row
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

type ListView struct {
	items       []ListItem
	hovered     int // -1 means nothing hovered
	selected    int // -1 means nothing selected, changes only on click
	style       Style
	hoverStyle  Style
	selectStyle Style // e.g. reversed background
}

func NewListView() *ListView {
	hoverStyle := DefaultStyle
	hoverStyle.Bold = true
	selectStyle := DefaultStyle
	selectStyle.Reversed = true

	return &ListView{
		hovered:     -1,
		selected:    -1,
		style:       DefaultStyle,
		hoverStyle:  hoverStyle,
		selectStyle: selectStyle,
	}
}

func (l *ListView) Append(text string, onClick func()) {
	l.items = append(l.items, ListItem{Text: text, OnClick: onClick})
}

func (l *ListView) MinSize() (int, int) {
	maxW := 10
	for _, it := range l.items {
		if w := runewidth.StringWidth(it.Text); w > maxW {
			maxW = w
		}
	}
	return maxW + 2, len(l.items) // a bit of padding + one row per item
}

func (l *ListView) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: l,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (l *ListView) Render(s Screen, rect Rect, style Style) {
	base := style.Merge(l.style)

	for i, item := range l.items {
		if i >= rect.H {
			break
		}

		st := base
		switch i {
		case l.selected:
			st = st.Merge(l.selectStyle)
		case l.hovered:
			st = st.Merge(l.hoverStyle)
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

func (l *ListView) OnMouseDown(x, y int) {
	if y >= 0 && y < len(l.items) {
		l.selected = y
		if l.items[y].OnClick != nil {
			l.items[y].OnClick()
		}
	}
}

func (l *ListView) OnMouseUp(x, y int) {}

func (l *ListView) OnMouseEnter() {}

func (l *ListView) OnMouseMove(rx, ry int) {
	if ry < 0 || ry >= len(l.items) {
		l.hovered = -1
		return
	}
	l.hovered = ry
}

func (l *ListView) OnMouseLeave() { l.hovered = -1 }

type TabView struct {
	labels   []*TabLabel
	bodys    []Element
	active   int
	Closable bool
}

type TabLabel struct {
	t       *TabView
	text    string
	hovered bool
}

func (l *TabLabel) OnMouseEnter() {
	l.hovered = true
}
func (l *TabLabel) OnMouseLeave() {
	l.hovered = false
}
func (l *TabLabel) OnMouseMove(rx, ry int) {
}

func (l *TabLabel) OnMouseUp(rx, ry int) {}
func (l *TabLabel) OnMouseDown(rx, ry int) {
	// Check if clicked on close 'x'
	labelWidth, _ := l.MinSize()
	if l.t.Closable && rx >= labelWidth-2 {
		// Close tab
		for i, label := range l.t.labels {
			if label == l {
				l.t.Close(i)
			}
		}
		return
	}

	for i, label := range l.t.labels {
		if label == l {
			l.t.SetActive(i)
			return
		}
	}
}

func (l *TabLabel) MinSize() (int, int) {
	return len(l.text) + 4, 1
}

func (l *TabLabel) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: l,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (l *TabLabel) Render(s Screen, rect Rect, style Style) {
	label := fmt.Sprintf(" %s ", l.text)
	var needClose bool
	st := style.Apply()
	if l == l.t.labels[l.t.active] {
		st = st.Underline(true).Bold(true)
		if l.t.Closable {
			needClose = true
		}
	} else if l.hovered {
		st = st.Reverse(true)
		if l.t.Closable {
			needClose = true
		}
	}
	if needClose {
		label += "x "
	}

	renderString(s, rect.X, rect.Y, rect.W, label, st)
}

func (l *TabLabel) FocusTarget() Element {
	if l.t.active < 0 || l.t.active >= len(l.t.bodys) {
		return l.t
	}
	return l.t.bodys[l.t.active]
}

func (l *TabLabel) OnFocus() {}
func (l *TabLabel) OnBlur()  {}

func (t *TabView) Append(label string, content Element) *TabView {
	tabLabel := &TabLabel{t: t, text: label}
	t.labels = append(t.labels, tabLabel)
	t.bodys = append(t.bodys, content)
	return t
}

func (t *TabView) Close(i int) {
	if i < 0 || i >= len(t.labels) {
		return
	}
	t.labels = append(t.labels[:i], t.labels[i+1:]...)
	t.bodys = append(t.bodys[:i], t.bodys[i+1:]...)
	if t.active >= len(t.labels) {
		t.active = len(t.labels) - 1
	}
	if t.active < 0 && len(t.labels) > 0 {
		t.active = 0
	}
}

func (t *TabView) MinSize() (int, int) {
	maxW, maxH := 0, 0
	for _, e := range t.bodys {
		w, h := e.MinSize()
		if w > maxW {
			maxW = w
		}
		if h > maxH {
			maxH = h
		}
	}
	return maxW, maxH + 1 // +1 for tab labels
}

func (t *TabView) Layout(x, y, w, h int) *LayoutNode {
	n := &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}

	// Layout tab labels on top
	offsetX := 0
	for _, l := range t.labels {
		labelW, labelH := l.MinSize()
		node := l.Layout(x+offsetX, y, labelW, labelH)
		n.Children = append(n.Children, node)
		offsetX += labelW
	}

	if t.active >= 0 && t.active < len(t.bodys) {
		node := t.bodys[t.active].Layout(x, y+1, w, h-1)
		n.Children = append(n.Children, node)
	}
	return n
}

func (t *TabView) Render(s Screen, rect Rect, style Style) {
	// do nothing, children render themselves
}

func (t *TabView) SetActive(i int) {
	if i >= 0 && i < len(t.bodys) {
		t.active = i
	}
}

func (t *TabView) FocusTarget() Element {
	if t.active < 0 || t.active >= len(t.bodys) {
		return t
	}
	return t.bodys[t.active]
}

func (t *TabView) OnFocus() {}
func (t *TabView) OnBlur()  {}
