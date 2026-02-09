// Package ui provides a lightweight text-based user interface toolkit built on top of tcell.
// It offers a clean event–state–render pipeline with basic UI components and layouts.
package ui

import (
	"fmt"
	"io"
	"log"
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
	// Size returns the minimum width and height required by the element.
	Size() (w, h int)

	// Layout builds the layout node for the element
	Layout(Rect) *Node

	// Draw draws the element's visual representation onto the screen.
	Draw(s Screen, rect Rect)
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
		n.Element.Draw(s, n.Rect)
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

// Contains returns true if (x, y) is inside the rectangle.
func (r Rect) Contains(x, y int) bool {
	return r.X <= x && x < r.X+r.W && r.Y <= y && y < r.Y+r.H
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

// DrawString draws the given string at (x, y) within width w using the specified style.
func DrawString(s Screen, x, y, w int, str string, style Style) {
	st := style.Apply()
	offset := 0
	for _, r := range str {
		rw := runewidth.RuneWidth(r)
		// Prevent wide characters from overflowing the boundary
		if rw > 0 && offset+rw > w {
			break
		}
		s.SetContent(x+offset, y, r, nil, st)
		if rw > 0 {
			offset += rw
		}
	}
}

// Manager manages the main event loop, rendering, and event dispatching.
type Manager struct {
	screen  Screen
	root    *Node // root node of the layout tree
	view    Element
	overlay *overlay

	// hit test
	clickX, clickY int
	focused        Element
	hover          Element

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
// This method should only be called after Start() has been invoked, as the screen
// is initialized during the Start() process.
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

// hitTest walks the layout tree to find the deepest matching element
// located at the given (x, y) in absolute coordinates, returns the element
// and the local coordinates within the node's rect.
func hitTest(n *Node, x, y int) (Element, int, int) {
	if n == nil || !n.Rect.Contains(x, y) {
		return nil, 0, 0
	}

	// Check children in reverse order (topmost first)
	for i := len(n.Children) - 1; i >= 0; i-- {
		child := n.Children[i]
		if e, lx, ly := hitTest(child, x, y); e != nil {
			return e, lx, ly
		}
	}

	return n.Element, x - n.Rect.X, y - n.Rect.Y
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

	var cursorColor string

	redraw := func() {
		if Theme.Cursor != cursorColor {
			screen.SetCursorStyle(tcell.CursorStyleDefault, tcell.GetColor(Theme.Cursor))
			cursorColor = Theme.Cursor
		}
		screen.Fill(' ', Style{}.Apply())
		m.Render()
		screen.Show()
	}

	redraw()

	for {
		ev := screen.PollEvent()
		dirty := false

		// Check if we should exit after receiving event
		select {
		case <-m.done:
			return nil
		default:
		}

		switch ev := ev.(type) {
		case *tcell.EventInterrupt:
			// waken by Refresh() or Stop()
			dirty = true
		case *tcell.EventResize:
			dirty = true
			screen.Sync()
		case *tcell.EventKey:
			m.handleKey(ev)
			dirty = true
		case *tcell.EventMouse:
			dirty = m.handleMouse(ev)
		}

		if dirty {
			redraw()
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

// Returns true if the event caused state changes that require a redraw.
func (m *Manager) handleMouse(ev *tcell.EventMouse) bool {
	x, y := ev.Position()
	hit, lx, ly := hitTest(m.root, x, y)
	if hit == nil {
		return false
	}

	dirty := m.updateHover(hit, lx, ly)

	switch ev.Buttons() {
	case tcell.ButtonPrimary:
		prevFocus := m.focused
		m.SetFocus(hit)
		if prevFocus != m.focused {
			dirty = true
		}
		m.clickX, m.clickY = x, y
		// mouse down
		if i, ok := hit.(Clickable); ok {
			i.OnMouseDown(lx, ly)
			dirty = true
		}
	case tcell.WheelUp:
		if i, ok := hit.(Scrollable); ok {
			i.OnScroll(-2)
			dirty = true
		}
	case tcell.WheelDown:
		if i, ok := hit.(Scrollable); ok {
			i.OnScroll(2)
			dirty = true
		}
	case tcell.ButtonNone:
		// mouse up
		if x == m.clickX && y == m.clickY {
			if i, ok := hit.(Clickable); ok {
				i.OnMouseUp(lx, ly)
				dirty = true
			}
		}
	}

	return dirty
}

// Returns true if hover state changed.
func (m *Manager) updateHover(e Element, lx, ly int) bool {
	changed := false
	if m.hover != e {
		if h, ok := m.hover.(Hoverable); ok {
			changed = true
			h.OnMouseLeave()
		}
		if h, ok := e.(Hoverable); ok {
			changed = true
			h.OnMouseEnter()
		}
		m.hover = e
	}

	if h, ok := e.(Hoverable); ok {
		h.OnMouseMove(lx, ly)
	}

	return changed
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
