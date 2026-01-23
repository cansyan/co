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

	// Layout builds the layout node for the element
	Layout(Rect) *Node

	// Render draws the element's visual representation onto the screen.
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

// Node represents a node in the layout/render tree.
type Node struct {
	Element  Element
	Rect     Rect
	Children []*Node
}

func (n *Node) Draw(s Screen) {
	if n.Element != nil {
		n.Element.Render(s, n.Rect)
	}
	for _, child := range n.Children {
		if child == nil {
			continue
		}
		child.Draw(s)
	}
}

// Find searches the tree for a node containing the target element.
func (n *Node) Find(target Element) *Node {
	if n == nil || target == nil {
		return nil
	}
	if n.Element == target {
		return n
	}
	for _, child := range n.Children {
		if child == nil {
			continue
		}
		if res := child.Find(target); res != nil {
			return res
		}
	}
	return nil
}

// Debug returns a string representation of the tree structure
func (n *Node) Debug() string {
	var sb strings.Builder
	var dump func(node *Node, prefix string, isLast bool, isRoot bool)
	dump = func(node *Node, prefix string, isLast bool, isRoot bool) {
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

func (t *Text) MinSize() (int, int) { return runewidth.StringWidth(t.Content), 1 }
func (t *Text) Layout(r Rect) *Node {
	return &Node{
		Element: t,
		Rect:    r,
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

// NewButton creates a new Button with the given label and click handler.
func NewButton(text string, onClick func()) *Button {
	return &Button{Text: text, OnClick: onClick}
}

func (b *Button) MinSize() (int, int) { return runewidth.StringWidth(b.Text) + 2, 1 }
func (b *Button) Layout(r Rect) *Node {
	return &Node{
		Element: b,
		Rect:    r,
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

// TextInput is a single-line text input field.
// The zero value for TextInput is ready to use.
type TextInput struct {
	text    []rune
	cursor  int // cursor position; also selection end
	anchor  int // selection start (rune index)
	pressed bool
	focused bool

	Placeholder string
	OnChange    func()
	OnCommit    func(string) // called when Enter is pressed, with current text
	Style       Style
}

// Text returns the current text content
func (t *TextInput) Text() string {
	return string(t.text)
}

func (t *TextInput) SetText(s string) {
	t.text = []rune(s)
	t.cursor = len(t.text)
	t.anchor = t.cursor
	if t.OnChange != nil {
		t.OnChange()
	}
}

func (t *TextInput) MinSize() (int, int) {
	return 10, 1
}

func (t *TextInput) Layout(r Rect) *Node {
	return &Node{
		Element: t,
		Rect:    r,
	}
}

func (t *TextInput) Render(s Screen, rect Rect) {
	if t.focused && t.cursor < rect.W {
		s.ShowCursor(rect.X+t.cursor, rect.Y)
	}
	// placeholder
	if len(t.text) == 0 {
		DrawString(s, rect.X, rect.Y, rect.W, t.Placeholder, Theme.Syntax.Comment.Apply())
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
		if t.OnChange != nil {
			t.OnChange()
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
		if t.OnChange != nil {
			t.OnChange()
		}
	case tcell.KeyEnter:
		if t.OnCommit != nil {
			t.OnCommit(string(t.text))
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
		if t.OnChange != nil {
			t.OnChange()
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

/*
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

func (tv *TextViewer) Layout(r Rect) *LayoutNode {
	return &LayoutNode{
		Element: tv,
		Rect:    r,
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
*/

// List displays a vertical list of items.
// The zero value is ready to use.
type List struct {
	Items    []ListItem
	Index    int // current selected index, -1 means none
	OnSelect func(ListItem)

	// no hover state, to avoid confusion with selection
}

type ListItem struct {
	Name  string
	Value any
}

func (l *List) MinSize() (int, int) {
	maxW := 10
	for _, it := range l.Items {
		if w := runewidth.StringWidth(it.Name); w > maxW {
			maxW = w
		}
	}
	return maxW + 2, len(l.Items)
}

func (l *List) Layout(r Rect) *Node {
	return &Node{
		Element: l,
		Rect:    r,
	}
}

func (l *List) Render(s Screen, rect Rect) {
	for i, item := range l.Items {
		if i >= rect.H {
			break
		}

		var st Style
		if l.Index == i {
			st.BG = Theme.Selection
		}

		label := fmt.Sprintf(" %s ", item.Name)
		w := runewidth.StringWidth(label)
		if w > rect.W {
			label = runewidth.Truncate(label, rect.W, "…")
		} else {
			label = runewidth.FillRight(label, rect.W)
		}
		DrawString(s, rect.X, rect.Y+i, rect.W, label, st.Apply())
	}
}

func (l *List) OnMouseDown(x, y int) {
	if y >= 0 && y < len(l.Items) {
		l.Index = y
		if l.OnSelect != nil {
			l.OnSelect(l.Items[y])
		}
	}
}

func (l *List) OnMouseUp(x, y int) {}

func (l *List) OnMouseMove(x, y int) {
	if y >= 0 && y < len(l.Items) {
		l.Index = y
	} else {
		l.Index = -1
	}
}

func (l *List) OnMouseEnter() {}
func (l *List) OnMouseLeave() { l.Index = -1 }
func (l *List) OnFocus()      {}
func (l *List) OnBlur()       {}

func (l *List) Append(item ListItem) {
	l.Items = append(l.Items, item)
}

func (l *List) Clear() {
	l.Items = nil
	l.Index = -1
}

func (l *List) Len() int {
	return len(l.Items)
}

func (l *List) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyCtrlP:
		l.Prev()
	case tcell.KeyDown, tcell.KeyCtrlN:
		l.Next()
	case tcell.KeyEnter:
		l.Activate()
	default:
		return false
	}
	return true
}

func (l *List) Next() {
	if len(l.Items) == 0 {
		return
	}
	l.Index = (l.Index + 1) % len(l.Items)
}

func (l *List) Prev() {
	if len(l.Items) == 0 {
		return
	}
	l.Index = (l.Index - 1 + len(l.Items)) % len(l.Items)
}

func (l *List) Activate() {
	if l.Index >= 0 && l.Index < len(l.Items) {
		if l.OnSelect != nil {
			l.OnSelect(l.Items[l.Index])
		}
	}
}

// Divider is a simple horizontal or vertical line separator.
// It can be used within container layouts like HStack and VStack.
type Divider struct {
	vertical bool
}

func (d *Divider) MinSize() (w, h int) { return 1, 1 }
func (d *Divider) Layout(r Rect) *Node {
	return &Node{
		Element: d,
		Rect:    r,
	}
}
func (d *Divider) Render(s Screen, rect Rect) {
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

type empty struct{}

func (e empty) MinSize() (int, int) { return 0, 0 }
func (e empty) Layout(r Rect) *Node {
	return &Node{Element: e, Rect: r}
}
func (e empty) Render(Screen, Rect) {}

// Manager manages the main event loop, rendering, and event dispatching.
type Manager struct {
	screen  Screen
	root    *Node // root node of the layout tree
	view    Element
	overlay *overlay

	// hit test
	clickPoint Point
	focused    Element
	hover      Element

	bindings map[string]func()
	done     chan struct{}
}

func NewManager() *Manager {
	return &Manager{
		done:     make(chan struct{}),
		bindings: make(map[string]func()),
	}
}

// Screen returns the tcell Screen instance for direct access to terminal features.
func (m *Manager) Screen() Screen {
	return m.screen
}

// Render builds the layout tree then draw it to the screen.
func (m *Manager) Render() {
	w, h := m.screen.Size()
	rect := Rect{X: 0, Y: 0, W: w, H: h}

	m.root = m.view.Layout(rect)
	if m.overlay != nil {
		m.root.Children = append(m.root.Children, m.overlay.Layout(rect))
	}

	m.root.Draw(m.screen)
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
func hitTest(n *Node, p Point) (Element, Point) {
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

func (m *Manager) SetFocus(e Element) {
	if e == m.focused {
		return
	}

	prev := m.focused
	defer func() {
		Logger.Printf("Focus changed: %T -> %T", prev, m.focused)
	}()

	m.blurCurrent()

	if e == nil {
		m.focused = nil
		return
	}

	e = m.resolveFocus(e)
	if fe, ok := e.(Focusable); ok {
		fe.OnFocus()
		m.focused = e
		// clear overlay if it lost focus
		if m.overlay != nil {
			overlayNode := m.root.Find(m.overlay)
			if overlayNode != nil && overlayNode.Find(e) == nil {
				m.overlay = nil
			}
		}
	} else {
		m.focused = nil
	}
}

func (m *Manager) blurCurrent() {
	if m.focused == nil {
		return
	}
	if f, ok := m.focused.(Focusable); ok {
		f.OnBlur()
	}
	m.screen.HideCursor()
}

func (m *Manager) resolveFocus(e Element) Element {
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
func (m *Manager) Refresh() {
	// sends an empty event, wakes screen.PollEvent()
	m.screen.PostEvent(tcell.NewEventInterrupt(nil))
}

// Start starts the main event loop
func (m *Manager) Start(view Element) error {
	m.view = view
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	m.screen = screen

	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.EnableMouse()

	redraw := func() {
		screen.SetCursorStyle(tcell.CursorStyleSteadyBar, tcell.GetColor(Theme.Cursor))
		screen.Fill(' ', Style{}.Apply())
		m.Render()
		screen.Show()
	}

	for {
		// Redraw on every event to keep things simple and clear
		redraw()

		ev := screen.PollEvent()

		// Check if we should exit after receiving event
		select {
		case <-m.done:
			return nil
		default:
		}

		switch ev := ev.(type) {
		case *tcell.EventInterrupt:
			// waken by Refresh() or Stop()
		case *tcell.EventResize:
			redraw()
			screen.Sync()
		case *tcell.EventKey:
			m.handleKey(ev)
		case *tcell.EventMouse:
			m.handleMouse(ev)
		}
	}
}

// Stop stops the main event loop and cleans up resources.
// Safe to call multiple times.
func (m *Manager) Stop() {
	select {
	case <-m.done:
		// Already closed, do nothing
		return
	default:
		// Still open, close it
		close(m.done)
		// Wake up the event loop if it's blocked in PollEvent
		if m.screen != nil {
			m.screen.PostEvent(tcell.NewEventInterrupt(nil))
		}
	}
}

// BindKey bind the key to the action globally,
// key should be form of "ctrl+c".
func (m *Manager) BindKey(key string, action func()) {
	if key == "" || action == nil {
		return
	}
	key = strings.ToLower(key)
	m.bindings[key] = action
}

func (m *Manager) handleKey(ev *tcell.EventKey) {
	Logger.Printf("key %s", ev.Name())
	// 1. Give the focused element first chance to handle the key event
	if m.focused != nil {
		if h, ok := m.focused.(KeyHandler); ok {
			if h.HandleKey(ev) {
				return
			}
		}
	}

	// 2. Framework-level automatic dismissal
	if ev.Key() == tcell.KeyESC && m.overlay != nil {
		m.CloseOverlay()
		return
	}

	// 3. Fallback to global bindings
	key := strings.ToLower(ev.Name())
	if action, ok := m.bindings[key]; ok {
		action()
		return
	}
}

// hover -> mouse down -> focus -> mouse up -> scroll
func (m *Manager) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	hit, local := hitTest(m.root, Point{X: x, Y: y})
	if hit == nil {
		return
	}
	lx, ly := local.X, local.Y

	m.updateHover(hit, lx, ly)

	switch ev.Buttons() {
	case tcell.ButtonPrimary:
		m.SetFocus(hit)
		m.clickPoint = Point{X: x, Y: y}
		// mouse down
		if i, ok := hit.(Clickable); ok {
			i.OnMouseDown(lx, ly)
		}
	case tcell.WheelUp:
		if i, ok := hit.(Scrollable); ok {
			i.OnScroll(-2)
		}
	case tcell.WheelDown:
		if i, ok := hit.(Scrollable); ok {
			i.OnScroll(2)
		}
	default:
		// mouse up
		if x == m.clickPoint.X && y == m.clickPoint.Y {
			if i, ok := hit.(Clickable); ok {
				i.OnMouseUp(lx, ly)
			}
		}
	}
}

func (m *Manager) updateHover(e Element, lx, ly int) {
	if m.hover != e {
		if h, ok := m.hover.(Hoverable); ok {
			h.OnMouseLeave()
		}
		if h, ok := e.(Hoverable); ok {
			h.OnMouseEnter()
		}
		m.hover = e
	}

	if h, ok := e.(Hoverable); ok {
		h.OnMouseMove(lx, ly)
	}
}

// Overlay displays an overlay element over the main layout
// with the given alignment, and sets focus to it.
func (m *Manager) Overlay(e Element, align string) {
	m.overlay = &overlay{
		child: e,
		align: align,
	}
	if m.focused != nil {
		m.overlay.prevFocus = m.focused
	}
	m.SetFocus(e)
}

// CloseOverlay dismiss the overlay, restore previous focus.
func (m *Manager) CloseOverlay() {
	if m.overlay != nil {
		m.SetFocus(m.overlay.prevFocus)
	}
	m.overlay = nil
}
