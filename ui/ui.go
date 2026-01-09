// Package ui provides a lightweight text-based user interface toolkit built on top of tcell.
// It offers a clean event–state–render pipeline with basic UI components and layouts.
package ui

import (
	"fmt"
	"io"
	"log"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

type Screen = tcell.Screen

// Logger is intended for debugging,
// discards all logs by default, until configured otherwise.
var Logger = log.New(io.Discard, "", 0)

// Element is the interface implemented by all UI elements.
type Element interface {
	// MinSize returns the minimum width and height required by the element.
	MinSize() (w, h int)

	// Layout computes the geometry and constructs the render tree for the element.
	//
	// Responsibilities:
	// 1. Geometry Calculation: Determine the final position and size for itself and its children.
	// 2. Render Tree Construction: Return a LayoutNode that maps the calculated Rect
	//    to the Element for the rendering pipeline.
	//
	// Important:
	// Decorators or Containers MUST set the 'Element' field of the returned LayoutNode
	// to themselves (the decorator instance) rather than their child. Failing to do so
	// will cause the rendering pipeline to skip the decorator's Render() method.
	Layout(x, y, w, h int) *LayoutNode

	// Render draws the element's visual representation onto the screen.
	// It is called by the framework during the paint phase, using the Rect
	// calculated during the Layout phase.
	Render(s Screen, rect Rect)
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
	// delta dy > 0 means scrolling up, dy < 0 means scrolling down.
	OnScroll(dy int)
}

// Focusable represents an element that can receive focus.
type Focusable interface {
	OnFocus()
	OnBlur()
}

// FocusDelegator allows an element to delegate focus to another element.
type FocusDelegator interface {
	FocusTarget() Element
}

// KeyHandler represents an element that can handle keyboard events.
type KeyHandler interface {
	HandleKey(ev *tcell.EventKey) bool
}

type LayoutNode struct {
	Element  Element
	Rect     Rect
	Children []*LayoutNode
}

func NewLayoutNode(e Element, x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: e,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

// Debug returns a string representation of the tree structure
func (n *LayoutNode) Debug() string {
	var sb strings.Builder
	var dump func(node *LayoutNode, prefix string, isLast bool, isRoot bool)
	dump = func(node *LayoutNode, prefix string, isLast bool, isRoot bool) {
		if node == nil {
			return
		}

		// Determine the connector symbol for the current line
		var marker string
		if isRoot {
			marker = ""
		} else if isLast {
			marker = "└── "
		} else {
			marker = "├── "
		}

		typeName := fmt.Sprintf("%T", node.Element)
		if idx := strings.LastIndex(typeName, "."); idx != -1 {
			typeName = typeName[idx+1:]
		}
		fmt.Fprintf(&sb, "%s%s%s: %+v\n", prefix, marker, typeName, node.Rect)

		// the indentation prefix for child nodes
		var newPrefix string
		if isRoot {
			newPrefix = ""
		} else {
			if isLast {
				// Last child, stop drawing vertical lines
				newPrefix = prefix + "    "
			} else {
				// Middle child, continue drawing vertical guide lines
				newPrefix = prefix + "│   "
			}
		}

		for i, child := range node.Children {
			isLastChild := i == len(node.Children)-1
			dump(child, newPrefix, isLastChild, false)
		}
	}
	dump(n, "", false, true)
	return sb.String()
}

type Rect struct {
	X, Y, W, H int
}

type Style struct {
	FG            string // foreground
	BG            string // background
	FontBold      bool
	FontItalic    bool
	FontUnderline bool
}

// Apply convert Style to tcell type, uses current color theme by default.
func (s Style) Apply() tcell.Style {
	st := tcell.StyleDefault
	if s.FG != "" {
		st = st.Foreground(tcell.GetColor(s.FG))
	} else {
		st = st.Foreground(tcell.GetColor(Theme.Foreground))
	}
	if s.BG != "" {
		st = st.Background(tcell.GetColor(s.BG))
	} else {
		st = st.Background(tcell.GetColor(Theme.Background))
	}
	if s.FontBold {
		st = st.Bold(true)
	}
	if s.FontItalic {
		st = st.Italic(true)
	}
	if s.FontUnderline {
		st = st.Underline(true)
	}
	return st
}

func (s Style) Merge(parent Style) Style {
	ns := s
	if ns.BG == "" {
		ns.BG = parent.BG
	}
	if ns.FG == "" {
		ns.FG = parent.FG
	}

	ns.FontBold = ns.FontBold || parent.FontBold
	ns.FontItalic = ns.FontItalic || parent.FontItalic
	ns.FontUnderline = ns.FontUnderline || parent.FontUnderline
	return ns
}

func DrawString(s Screen, x, y, w int, str string, style tcell.Style) {
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
	Style   Style
	Content string
}

func NewText(text string) *Text {
	return &Text{Content: text}
}

// SetBold enables bold styling (chainable)
func (t *Text) SetBold() *Text { t.Style.FontBold = true; return t }

// SetItalic enables italic styling (chainable)
func (t *Text) SetItalic() *Text { t.Style.FontItalic = true; return t }

// SetUnderline enables underline styling (chainable)
func (t *Text) SetUnderline() *Text { t.Style.FontUnderline = true; return t }

// SetForeground sets the foreground color (chainable)
func (t *Text) SetForeground(color string) *Text {
	t.Style.FG = color
	return t
}

// SetBackground sets the background color (chainable)
func (t *Text) SetBackground(color string) *Text {
	t.Style.BG = color
	return t
}

func (t *Text) MinSize() (int, int) { return runewidth.StringWidth(t.Content), 1 }
func (t *Text) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (t *Text) Render(s Screen, rect Rect) {
	DrawString(s, rect.X, rect.Y, rect.W, t.Content, t.Style.Apply())
}

type Button struct {
	Style   Style
	Text    string
	OnClick func()

	hovered    bool
	pressed    bool
	NoFeedback bool // disables visual feedback for hover/press states
}

// NewButton creates a new button element with the given label.
func NewButton(text string, onClick func()) *Button {
	return &Button{Text: text, OnClick: onClick}
}

func (b *Button) MinSize() (int, int) { return runewidth.StringWidth(b.Text) + 2, 1 }
func (b *Button) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: b,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (b *Button) Render(s Screen, rect Rect) {
	st := b.Style
	if !b.NoFeedback && b.hovered {
		st.BG = Theme.Hover
	}
	if !b.NoFeedback && b.pressed {
		st.BG = Theme.Selection
	}
	label := " " + b.Text + " "
	DrawString(s, rect.X, rect.Y, rect.W, label, st.Apply())
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

// Trigger invokes the button's click action programmatically
func (b *Button) Trigger() {
	if b.OnClick != nil {
		b.OnClick()
	}
}

// SetForeground sets the foreground color (chainable)
func (b *Button) SetForeground(color string) *Button {
	b.Style.FG = color
	return b
}

// SetBackground sets the background color (chainable)
func (b *Button) SetBackground(color string) *Button {
	b.Style.BG = color
	return b
}

// TextInput is a single-line text input field.
// The zero value for TextInput is ready to use.
type TextInput struct {
	text        []rune
	cursor      int // cursor position; also selection end
	focused     bool
	Style       Style
	onChange    func()
	placeHolder string
	anchor      int // selection anchor (rune index)
	pressed     bool
	onCommit    func()
}

// Text returns the current text content
func (t *TextInput) Text() string {
	return string(t.text)
}

func (t *TextInput) SetText(s string) {
	t.text = []rune(s)
	t.cursor = len(t.text)
	t.anchor = t.cursor
	if t.onChange != nil {
		t.onChange()
	}
}

func (t *TextInput) OnChange(fn func()) *TextInput {
	t.onChange = fn
	return t
}

func (t *TextInput) OnCommit(fn func()) *TextInput {
	t.onCommit = fn
	return t
}

func (t *TextInput) SetPlaceholder(s string) {
	t.placeHolder = s
}

func (t *TextInput) MinSize() (int, int) {
	return 10, 1
}

func (t *TextInput) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (t *TextInput) Render(s Screen, rect Rect) {
	if t.focused && t.cursor < rect.W {
		s.ShowCursor(rect.X+t.cursor, rect.Y)
	}
	// placeholder
	if len(t.text) == 0 {
		DrawString(s, rect.X, rect.Y, rect.W, t.placeHolder, Theme.Syntax.Comment.Apply())
		return
	}

	start, end, hasSel := t.selection()
	baseStyle := t.Style.Apply()
	selStyle := t.Style.Merge(Style{BG: Theme.Selection}).Apply()

	xOffset := 0
	for i, r := range t.text {
		if xOffset >= rect.W {
			break
		}

		st := baseStyle
		if hasSel && i >= start && i < end {
			st = selStyle
		}

		s.SetContent(rect.X+xOffset, rect.Y, r, nil, st)
		xOffset += runewidth.RuneWidth(r)
	}

	// 如果文字不夠長，補足剩餘背景
	for x := xOffset; x < rect.W; x++ {
		s.SetContent(rect.X+x, rect.Y, ' ', nil, baseStyle)
	}
}

func (t *TextInput) OnFocus() { t.focused = true }
func (t *TextInput) OnBlur()  { t.focused = false }
func (t *TextInput) HandleKey(ev *tcell.EventKey) bool {
	resetSelection := true
	consumed := true
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
		start, end, ok := t.selection()
		if ok {
			t.text = slices.Delete(t.text, start, end)
			t.cursor = start
		} else if t.cursor > 0 {
			t.text = slices.Delete(t.text, t.cursor-1, t.cursor)
			t.cursor--
		}
		if t.onChange != nil {
			t.onChange()
		}
	case tcell.KeyRune:
		start, end, ok := t.selection()
		if ok {
			t.text = slices.Delete(t.text, start, end)
			t.cursor = start
		}
		r := ev.Rune()
		t.text = slices.Insert(t.text, t.cursor, r)
		t.cursor++
		if t.onChange != nil {
			t.onChange()
		}
	case tcell.KeyEnter:
		if t.onCommit != nil {
			t.onCommit()
		}
	default:
		consumed = false
	}

	if resetSelection && !t.pressed {
		t.anchor = t.cursor
	}
	return consumed
}

func (t *TextInput) OnMouseDown(x, y int) {
	if x < 0 {
		x = 0
	}
	if x > len(t.text) {
		x = len(t.text)
	}
	t.cursor = x
	if !t.pressed {
		t.pressed = true
		t.anchor = x
	}
}

func (t *TextInput) OnMouseMove(x, y int) {
	if t.pressed {
		t.cursor = t.clampCursor(x)
		if t.onChange != nil {
			t.onChange()
		}
	}
}

func (t *TextInput) OnMouseUp(x, y int) {
	t.pressed = false
}

func (t *TextInput) clampCursor(x int) int {
	if x < 0 {
		return 0
	}
	if x > len(t.text) {
		return len(t.text)
	}
	return x
}

func (t *TextInput) Select(start, end int) {
	t.anchor = start
	t.cursor = end
}

// 取得正規化後的選取範圍 (start <= end)
func (t *TextInput) selection() (int, int, bool) {
	if t.anchor == t.cursor {
		return 0, 0, false
	}
	start, end := t.anchor, t.cursor
	if start > end {
		start, end = end, start
	}
	return start, end, true
}

// TextViewer is a non-editable text viewer,
// supports multiple lines, scrolling and following tail.
type TextViewer struct {
	Style
	Lines    []string
	OffsetY  int
	AutoTail bool
	height   int
	onChange func()
}

func NewTextViewer(s string) *TextViewer {
	tv := &TextViewer{AutoTail: true}
	if s != "" {
		if s[len(s)-1] == '\n' {
			s = s[:len(s)-1]
		}
		tv.Lines = strings.Split(s, "\n")
	}
	return tv
}

func (tv *TextViewer) MinSize() (int, int) {
	return 25, 1 // let layout decide the height
}

func (tv *TextViewer) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: tv,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (tv *TextViewer) Render(s Screen, rect Rect) {
	tv.height = rect.H
	start := tv.OffsetY
	end := min(tv.OffsetY+rect.H, len(tv.Lines))
	y := rect.Y
	for i := start; i < end; i++ {
		DrawString(s, rect.X, y, rect.W, tv.Lines[i], tv.Style.Apply())
		y++
	}
}

func (tv *TextViewer) OnScroll(dy int) {
	old := tv.OffsetY
	if len(tv.Lines) <= tv.height {
		tv.OffsetY = 0
	} else if dy < 0 {
		// scroll down
		tv.OffsetY = max(tv.OffsetY+dy, 0)
	} else {
		// scroll up
		tv.OffsetY = min(tv.OffsetY+dy, len(tv.Lines)-tv.height)
	}

	// ones scroll and not at the end of file, stop following tail
	if tv.OffsetY >= len(tv.Lines)-tv.height {
		tv.AutoTail = true
	} else if tv.OffsetY != old {
		tv.AutoTail = false
	}

	if tv.onChange != nil {
		tv.onChange()
	}
}

func (tv *TextViewer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if p[len(p)-1] == '\n' {
		// do not append newline
		p = p[:len(p)-1]
	}
	lines := strings.Split(string(p), "\n")
	tv.Lines = append(tv.Lines, lines...)
	if tv.AutoTail {
		tv.OffsetY = max(0, len(tv.Lines)-tv.height)
	}
	if tv.onChange != nil {
		tv.onChange()
	}
	return len(p), nil
}

type ListView struct {
	Items    []ListItem
	hovered  int // -1 means nothing hovered
	Selected int // -1 means nothing selected, changes only on click
}

type ListItem struct {
	Label  string
	Action func()
}

func NewListView() *ListView {
	l := &ListView{
		hovered:  -1,
		Selected: -1,
	}
	return l
}

func (l *ListView) Append(text string, action func()) {
	l.Items = append(l.Items, ListItem{Label: text, Action: action})
}

func (l *ListView) Clear() {
	l.Items = nil
	l.hovered = -1
	l.Selected = -1
}

func (l *ListView) MinSize() (int, int) {
	maxW := 10
	for _, it := range l.Items {
		if w := runewidth.StringWidth(it.Label); w > maxW {
			maxW = w
		}
	}
	return maxW + 2, len(l.Items) // a bit of padding + one row per item
}

func (l *ListView) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: l,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (l *ListView) Render(s Screen, rect Rect) {
	for i, item := range l.Items {
		if i >= rect.H {
			break
		}

		var st Style
		switch i {
		case l.Selected:
			st.BG = Theme.Selection
		case l.hovered:
			st.BG = Theme.Hover
		}

		label := fmt.Sprintf(" %s ", item.Label)
		w := runewidth.StringWidth(label)
		if w > rect.W {
			label = runewidth.Truncate(label, rect.W, "…")
		} else {
			label = runewidth.FillRight(label, rect.W)
		}
		DrawString(s, rect.X, rect.Y+i, rect.W, label, st.Apply())
	}
}

func (l *ListView) OnMouseDown(x, y int) {
	if y >= 0 && y < len(l.Items) {
		l.Selected = y
		if l.Items[y].Action != nil {
			l.Items[y].Action()
		}
	}
}

func (l *ListView) OnMouseUp(x, y int) {}

func (l *ListView) OnMouseEnter() {}

func (l *ListView) OnMouseMove(rx, ry int) {
	if ry < 0 || ry >= len(l.Items) {
		l.hovered = -1
		return
	}
	l.hovered = ry
}

func (l *ListView) OnMouseLeave() { l.hovered = -1 }

func (l *ListView) OnFocus() {}
func (l *ListView) OnBlur()  {}

// SelectNext moves selection to the next item
func (l *ListView) SelectNext() {
	l.Selected = (l.Selected + 1) % len(l.Items)
}

// SelectPrev moves selection to the previous item
func (l *ListView) SelectPrev() {
	l.Selected = (l.Selected - 1 + len(l.Items)) % len(l.Items)
}

// Activate invokes the action of the currently selected item
func (l *ListView) Activate() {
	if len(l.Items) == 0 || l.Selected < 0 {
		return
	}
	if l.Items[l.Selected].Action != nil {
		l.Items[l.Selected].Action()
	}
}

func (l *ListView) HandleKey(ev *tcell.EventKey) bool {
	consumed := true
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyCtrlP:
		l.SelectPrev()
	case tcell.KeyDown, tcell.KeyCtrlN:
		l.SelectNext()
	case tcell.KeyEnter:
		l.Activate()
	default:
		consumed = false
	}
	return consumed
}

type divider struct {
	vertical bool
}

// Divider creates a horizontal or vertical divider line.
// Should be used inside HStack or VStack.
func Divider() *divider {
	return &divider{}
}

func (d *divider) MinSize() (w, h int) { return 1, 1 }
func (d *divider) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: d,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (d *divider) Render(s Screen, rect Rect) {
	style := Style{FG: Theme.Border}
	if !d.vertical {
		for i := range rect.W {
			s.SetContent(rect.X+i, rect.Y+rect.H-1, hLine, nil, style.Apply())
		}
	} else {
		for i := range rect.H {
			s.SetContent(rect.X+rect.W-1, rect.Y+i, vLine, nil, style.Apply())
		}
	}
}

type Empty struct{}

func (e Empty) MinSize() (int, int) { return 0, 0 }
func (e Empty) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{Element: e, Rect: Rect{X: x, Y: y, W: w, H: h}}
}
func (e Empty) Render(Screen, Rect) {}

// ---------------------------------------------------------------------
// APP RUNNER
// ---------------------------------------------------------------------

type App struct {
	screen  Screen
	root    Element // root element of the UI hierarchy to be rendered
	focused Element
	hover   Element
	tree    *LayoutNode // reflects the view hierarchy after last render
	done    chan struct{}

	clickPoint Point
	bindings   map[string]func()
	overlay    *overlay // for temporary display
}

func NewApp() *App {
	a := &App{
		done:     make(chan struct{}),
		bindings: make(map[string]func()),
	}
	return a
}

func drawTree(node *LayoutNode, s Screen) {
	if node == nil {
		return
	}

	node.Element.Render(s, node.Rect)
	for _, child := range node.Children {
		drawTree(child, s)
	}
}

// Render builds the layout tree then render it to the screen.
func (a *App) Render() {
	w, h := a.screen.Size()
	a.tree = a.root.Layout(0, 0, w, h)
	if o := a.overlay; o != nil {
		node := o.Layout(0, 0, w, h)
		a.tree.Children = append(a.tree.Children, node)
	}
	drawTree(a.tree, a.screen)
}

// Point represent a position in the screen coordinate.
// TODO: Point or bare (x, y) ?
type Point struct {
	X, Y int
}

func (p Point) In(r Rect) bool {
	return r.X <= p.X && p.X < r.X+r.W && r.Y <= p.Y && p.Y < r.Y+r.H
}

// hitTest walks the layout tree to find the deepest matching element
// located at the given point in absolute coordinates, returns the element
// and a point converted into the node's local coordinate space.
func hitTest(n *LayoutNode, p Point) (Element, Point) {
	if n == nil {
		return nil, Point{}
	}
	if !p.In(n.Rect) {
		return nil, Point{}
	}

	// Check children in reverse order (topmost first)
	for i := len(n.Children) - 1; i >= 0; i-- {
		child := n.Children[i]
		if e, local := hitTest(child, p); e != nil {
			return e, local
		}
	}

	return n.Element, Point{
		X: p.X - n.Rect.X,
		Y: p.Y - n.Rect.Y,
	}
}

func (a *App) SetFocus(e Element) {
	defer func(prev Element) {
		Logger.Printf("Focus changed: %T -> %T", prev, a.focused)
	}(a.focused)

	if e == nil {
		if a.focused != nil {
			if f, ok := a.focused.(Focusable); ok {
				f.OnBlur()
			}
			a.screen.HideCursor()
		}
		a.focused = nil
		return
	}

	if e == a.focused {
		return
	}

	if a.focused != nil {
		if f, ok := a.focused.(Focusable); ok {
			f.OnBlur()
		}
		a.screen.HideCursor()
	}

	e = a.resolveFocus(e)
	if fe, ok := e.(Focusable); ok {
		fe.OnFocus()
		a.focused = e
	} else {
		a.focused = nil
	}

	// If the overlay exists but the focused element is not part of it,
	// remove the overlay.
	if n := findNode(a.tree, a.overlay); n != nil {
		if findNode(n, e) == nil {
			a.overlay = nil
		}
	}
}

func findNode(n *LayoutNode, target Element) *LayoutNode {
	if n == nil || target == nil {
		return nil
	}
	if n.Element == target {
		return n
	}
	for _, child := range n.Children {
		if res := findNode(child, target); res != nil {
			return res
		}
	}
	return nil
}

func (a *App) resolveFocus(e Element) Element {
	visited := make(map[Element]bool)
	for {
		if visited[e] {
			Logger.Printf("Circular focus delegation detected for %T", e)
			return e
		}
		visited[e] = true

		f, ok := e.(FocusDelegator)
		if !ok {
			return e
		}
		t := f.FocusTarget()
		if t == e || t == nil {
			return e
		}
		e = t
	}
}

// Refresh requests a redraw of the UI
func (a *App) Refresh() {
	// sends an empty event, wakes screen.PollEvent()
	a.screen.PostEvent(tcell.NewEventInterrupt(nil))
}

// Run starts the main event loop
func (a *App) Run(root Element) error {
	a.root = root
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	a.screen = screen

	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.EnableMouse()

	draw := func() {
		screen.SetCursorStyle(tcell.CursorStyleSteadyBar, tcell.GetColor(Theme.Cursor))
		screen.Fill(' ', Style{}.Apply())
		a.Render()
		screen.Show()
	}

	for {
		select {
		case <-a.done:
			return nil
		default:
		}

		// Redraw on every event to keep things simple and clear
		draw()

		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventInterrupt:
			// waken by Refresh()
		case *tcell.EventResize:
			draw()
			screen.Sync()
		case *tcell.EventKey:
			a.handleKey(ev)
		case *tcell.EventMouse:
			a.handleMouse(ev)
		}
	}
}

// BindKey bind the key to the action globally,
// key should be form of "ctrl+c".
func (a *App) BindKey(key string, action func()) {
	if key == "" || action == nil {
		return
	}
	key = strings.ToLower(key)
	a.bindings[key] = action
}

func (a *App) handleKey(ev *tcell.EventKey) {
	Logger.Printf("key %s", ev.Name())
	// 1. Give the focused element first chance to handle the key event
	if a.focused != nil {
		if h, ok := a.focused.(KeyHandler); ok {
			if h.HandleKey(ev) {
				return
			}
		}
	}

	// 2. Framework-level automatic dismissal
	if ev.Key() == tcell.KeyESC && a.overlay != nil {
		a.CloseOverlay()
		return
	}

	// 3. Fallback to global bindings
	key := strings.ToLower(ev.Name())
	if action, ok := a.bindings[key]; ok {
		action()
		return
	}
}

// hover -> mouse down -> focus -> mouse up -> scroll
func (a *App) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	hit, local := hitTest(a.tree, Point{X: x, Y: y})
	if hit == nil {
		return
	}
	lx, ly := local.X, local.Y

	a.updateHover(hit, lx, ly)

	switch ev.Buttons() {
	case tcell.ButtonPrimary:
		a.SetFocus(hit)
		a.clickPoint = Point{X: x, Y: y}
		// mouse down
		if i, ok := hit.(Clickable); ok {
			i.OnMouseDown(lx, ly)
		}
	case tcell.WheelUp:
		if i, ok := hit.(Scrollable); ok {
			i.OnScroll(-1)
		}
	case tcell.WheelDown:
		if i, ok := hit.(Scrollable); ok {
			i.OnScroll(1)
		}
	default:
		// mouse up
		if x == a.clickPoint.X && y == a.clickPoint.Y {
			if i, ok := hit.(Clickable); ok {
				i.OnMouseUp(lx, ly)
			}
		}
	}
}

func (a *App) updateHover(e Element, lx, ly int) {
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
		h.OnMouseMove(lx, ly)
	}
}

func (a *App) Stop() {
	close(a.done)
}

// Overlay displays an overlay element over the main layout
// with the given alignment, and sets focus to it.
func (a *App) Overlay(e Element, align string) {
	a.overlay = &overlay{
		child: e,
		align: align,
	}
	if a.focused != nil {
		a.overlay.prevFocus = a.focused
	}
	a.SetFocus(e)
}

// CloseOverlay dismiss the overlay, restore previous focus.
func (a *App) CloseOverlay() {
	if a.overlay != nil {
		a.SetFocus(a.overlay.prevFocus)
	}
	a.overlay = nil
}
