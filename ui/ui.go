// Package ui provides a lightweight text-based user interface toolkit built on top of tcell.
// It offers a clean event–state–render pipeline with basic UI components and layouts.
package ui

import (
	"fmt"
	"log"
	"math"
	"slices"
	"strconv"
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
	// delta dy > 0 means scrolling up, dy < 0 means scrolling down.
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
func drawString(s Screen, x, y, w int, str string, style tcell.Style) {
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

type Empty struct{}

func (e Empty) MinSize() (int, int)               { return 0, 0 }
func (e Empty) Layout(x, y, w, h int) *LayoutNode { return nil }
func (e Empty) Render(Screen, Rect, Style)        {}

type TextElem struct {
	content string
	style   Style
}

func Text(c string) *TextElem { return &TextElem{content: c, style: DefaultStyle} }

func (t *TextElem) SetText(c string) { t.content = c }

func (t *TextElem) Bold() *TextElem      { t.style.Bold = true; return t }
func (t *TextElem) Italic() *TextElem    { t.style.Italic = true; return t }
func (t *TextElem) Underline() *TextElem { t.style.Underline = true; return t }
func (t *TextElem) Foreground(c string) *TextElem {
	t.style.FG = tcell.ColorNames[c]
	return t
}
func (t *TextElem) Background(c string) *TextElem {
	t.style.BG = tcell.ColorNames[c]
	return t
}

func (t *TextElem) MinSize() (int, int) { return len(t.content), 1 }
func (t *TextElem) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (t *TextElem) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(t.style).Apply()
	drawString(s, rect.X, rect.Y, rect.W, t.content, st)
}

type ButtonElem struct {
	Label   string
	style   Style
	OnClick func()
	hovered bool
	pressed bool
}

// Button creates a new button element with the given label.
func Button(label string) *ButtonElem {
	return &ButtonElem{Label: label, style: DefaultStyle}
}
func (b *ButtonElem) Foreground(c string) *ButtonElem {
	b.style.FG = tcell.ColorNames[c]
	return b
}
func (b *ButtonElem) Background(c string) *ButtonElem {
	b.style.BG = tcell.ColorNames[c]
	return b
}

func (b *ButtonElem) MinSize() (int, int) { return runewidth.StringWidth(b.Label) + 2, 1 }
func (b *ButtonElem) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: b,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (b *ButtonElem) Render(s Screen, rect Rect, style Style) {
	st := style.Merge(b.style).Apply()
	if b.hovered {
		st = st.Underline(true).Bold(true)
	}
	if b.pressed {
		st = st.Reverse(true)
	}
	label := " " + b.Label + " "
	drawString(s, rect.X, rect.Y, rect.W, label, st)
}

func (b *ButtonElem) OnMouseEnter() { b.hovered = true }

func (b *ButtonElem) OnMouseLeave() {
	b.hovered = false
	b.pressed = false // cancel
}

func (b *ButtonElem) OnMouseMove(rx, ry int) {}

func (b *ButtonElem) OnMouseDown(x, y int) {
	b.pressed = true
}

func (b *ButtonElem) OnMouseUp(x, y int) {
	if b.pressed && b.hovered {
		// real click
		if b.OnClick != nil {
			b.OnClick()
		}
	}
	b.pressed = false
}

// TextInput is a single-line editable text input field.
type TextInput struct {
	text     []rune
	cursor   int
	active   bool
	style    Style
	onChange func(string)
}

// func NewTextInput() *TextInput {
// 	return &TextInput{
// 		style: DefaultStyle,
// 	}
// }

func (t *TextInput) String() string {
	return string(t.text)
}

func (t *TextInput) SetText(s string) {
	t.text = []rune(s)
	t.cursor = len(t.text)
}

func (t *TextInput) OnChange(fn func(string)) *TextInput {
	t.onChange = fn
	return t
}

func (t *TextInput) Foreground(c string) *TextInput {
	t.style.FG = tcell.ColorNames[c]
	return t
}
func (t *TextInput) Background(c string) *TextInput {
	t.style.BG = tcell.ColorNames[c]
	return t
}

func (t *TextInput) MinSize() (int, int) { return 10, 1 }

func (t *TextInput) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (t *TextInput) Render(s Screen, rect Rect, style Style) {
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

func (t *TextInput) FocusTarget() Element { return t }
func (t *TextInput) OnFocus()             { t.active = true }
func (t *TextInput) OnBlur()              { t.active = false }

func (t *TextInput) HandleKey(ev *tcell.EventKey) {
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

func (t *TextInput) OnMouseDown(x, y int) {
	if x < 0 {
		x = 0
	}
	if x > len(t.text) {
		x = len(t.text)
	}
	t.cursor = x
}

// TextViewer is a non-editable text viewer,
// supports multiple lines, scrolling and following tail.
// It can be used for log panel or debug.
type TextViewer struct {
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

func (tv *TextViewer) Render(s Screen, rect Rect, style Style) {
	tv.height = rect.H
	start := tv.OffsetY
	end := tv.OffsetY + rect.H
	if end > len(tv.Lines) {
		end = len(tv.Lines)
	}
	y := rect.Y
	for i := start; i < end; i++ {
		drawString(s, rect.X, y, rect.W, tv.Lines[i], style.Apply())
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

// OnChange register a callback function that will be called on content changed
func (tv *TextViewer) OnChange(f func()) { tv.onChange = f }

// TextEditor is a multi-line editable text area.
type TextEditor struct {
	content      [][]rune // simple 2D slice of runes, avoid over-engineering
	row          int      // Current line index
	col          int      // Cursor column index (rune index)
	offsetY      int      // Vertical scroll offset
	focused      bool
	style        Style
	viewH        int // last rendered height
	lineNumWidth int
	onChange     func()
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

	// Dynamic width calculation (for proper right-justification)
	numLines := len(t.content)
	if numLines == 0 {
		numLines = 1
	}
	actualNumDigits := len(strconv.Itoa(numLines))
	lineNumWidth := actualNumDigits + 2
	t.lineNumWidth = lineNumWidth
	lineNumStyle := st.Reverse(false).Foreground(tcell.ColorSilver)

	contentX := rect.X + lineNumWidth
	contentW := rect.W - lineNumWidth
	if contentW <= 0 {
		return
	}

	var cursorX, cursorY int
	cursorFound := false
	// Loop over visible rows
	for i := range rect.H {
		row := i + t.offsetY
		if row >= len(t.content) {
			break
		}

		line := t.content[row]
		if row == t.row {
			cursorFound = true
			cursorX = contentX + runewidth.StringWidth(string(line[:t.col]))
			cursorY = rect.Y + i
		}

		// Render Line Number (UNCONDITIONAL)
		lineNum := row + 1
		numStr := fmt.Sprintf("%*d ", lineNumWidth-1, lineNum)
		drawString(s, rect.X, rect.Y+i, lineNumWidth, numStr, lineNumStyle)

		// Render Line Content
		drawString(s, contentX, rect.Y+i, contentW, string(line), st)
	}

	// Place the cursor
	if t.focused && cursorFound {
		s.ShowCursor(cursorX, cursorY)
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
		newLine := slices.Clone(tail)
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

	clickedX := max(x-t.lineNumWidth, 0)

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
	if len(t.content) <= t.viewH {
		t.offsetY = 0
	} else if dy < 0 {
		// scroll down
		t.offsetY = max(t.offsetY+dy, 0)
	} else {
		// scroll up
		t.offsetY = min(t.offsetY+dy, len(t.content)-t.viewH)
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

func (t *TextEditor) Grow(weight ...int) *grower { return Grow(t, weight...) }

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

	// First pass: measure children
	totalH := 0
	totalWeight := 0
	for _, child := range v.children {
		if i, ok := child.(*grower); ok {
			totalWeight += i.weight
		} else {
			_, ch := child.MinSize()
			totalH += ch
		}
	}

	// Compute remaining space
	remain := max(h-totalH-v.spacing*(len(v.children)-1), 0)
	share := float64(remain) / float64(totalWeight)

	// Second pass: layout children
	used := 0
	for i, child := range v.children {
		if d, ok := child.(*divider); ok {
			d.vertical = false
		}
		_, ch := child.MinSize()
		if g, ok := child.(*grower); ok && totalWeight > 0 {
			ch = int(math.Ceil(float64(g.weight) * share))
			if ch > remain {
				ch = remain
			}
			remain -= ch
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

func (v *vstack) Grow(weight ...int) *grower { return Grow(v, weight...) }

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
	totalWidth := 0
	totalWeight := 0
	for _, child := range hs.children {
		if g, ok := child.(*grower); ok {
			totalWeight += g.weight
		} else {
			cw, _ := child.MinSize()
			totalWidth += cw
		}
	}

	// Compute remaining space
	remain := max(w-totalWidth-hs.spacing*(len(hs.children)-1), 0)
	share := float64(remain) / float64(totalWeight)

	// Second pass: layout children
	used := 0
	for i, child := range hs.children {
		if div, ok := child.(*divider); ok {
			div.vertical = true
		}
		cw, _ := child.MinSize()
		if g, ok := child.(*grower); ok && totalWeight > 0 {
			cw = int(math.Ceil(float64(g.weight) * share))
			if cw > remain {
				cw = remain
			}
			remain -= cw
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

func (hs *hstack) Grow(weight ...int) *grower {
	return Grow(hs, weight...)
}

type grower struct {
	child  Element
	weight int
}

// Grow expands the given element to occupy the remaining available space.
// The optional weight (default 1) controls how extra space is distributed
// among siblings inside an HStack or VStack.
func Grow(e Element, weight ...int) *grower {
	w := 1
	if len(weight) > 0 {
		w = weight[0]
	}
	return &grower{child: e, weight: w}
}

func (g *grower) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: g,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
		Children: []*LayoutNode{
			g.child.Layout(x, y, w, h),
		},
	}
}

func (g *grower) MinSize() (int, int) {
	return g.child.MinSize()
}

func (g *grower) Render(s Screen, rect Rect, style Style) {
	// do nothing
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
func (d *divider) Render(s Screen, rect Rect, style Style) {
	st := style.Apply()
	if !d.vertical {
		for i := range rect.W {
			s.SetContent(rect.X+i, rect.Y+rect.H-1, '-', nil, st)
		}
	} else {
		for i := range rect.H {
			s.SetContent(rect.X+rect.W-1, rect.Y+i, '|', nil, st)
		}
	}
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

// Spacer fills the remaining space between siblings inside an HStack or VStack.
var Spacer = Grow(Empty{})

// Frame is a fixed-size wrapper that always reports the size given by W and H.
// It layouts exactly one child element and forces the child to fill the frame.
// Rendering is a no-op; the frame exists purely as a layout boundary.
type Frame struct {
	W, H  int // 0 means using child's min size
	Child Element
}

func (f *Frame) MinSize() (int, int) {
	w, h := f.W, f.H
	if w == 0 {
		cw, _ := f.Child.MinSize()
		w = cw
	}
	if h == 0 {
		_, ch := f.Child.MinSize()
		h = ch
	}
	return w, h
}

func (f *Frame) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element:  f,
		Rect:     Rect{X: x, Y: y, W: w, H: h},
		Children: []*LayoutNode{f.Child.Layout(x, y, w, h)},
	}
}

func (f *Frame) Render(s Screen, rect Rect, style Style) {
	// no-op
}

// ---------------------------------------------------------------------
// Containers (View)
// ---------------------------------------------------------------------

type ListView struct {
	items    []ListItem
	hovered  int // -1 means nothing hovered
	selected int // -1 means nothing selected, changes only on click
}

type ListItem struct {
	label   string
	OnClick func()
}

func NewListView() *ListView {
	return &ListView{
		hovered:  -1,
		selected: -1,
	}
}

func (l *ListView) Append(text string, onClick func()) {
	l.items = append(l.items, ListItem{label: text, OnClick: onClick})
}

func (l *ListView) MinSize() (int, int) {
	maxW := 10
	for _, it := range l.items {
		if w := runewidth.StringWidth(it.label); w > maxW {
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
	hoverStyle := DefaultStyle
	hoverStyle.Bold = true
	hoverStyle.Underline = true
	selectStyle := DefaultStyle
	selectStyle.Reversed = true

	for i, item := range l.items {
		if i >= rect.H {
			break
		}

		st := style
		switch i {
		case l.selected:
			st = st.Merge(selectStyle)
		case l.hovered:
			st = st.Merge(hoverStyle)
		}

		label := fmt.Sprintf(" %s ", item.label)
		if w := runewidth.StringWidth(label); w > rect.W {
			label = runewidth.Truncate(label, rect.W, "…")
		}
		drawString(s, rect.X, rect.Y+i, rect.W, label, st.Apply())
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

type TabItem struct {
	t       *TabView
	label   string
	body    Element
	hovered bool
}

func (ti *TabItem) OnMouseEnter() {
	ti.hovered = true
}
func (ti *TabItem) OnMouseLeave() {
	ti.hovered = false
}
func (ti *TabItem) OnMouseMove(rx, ry int) {
}

func (ti *TabItem) OnMouseUp(rx, ry int) {}
func (ti *TabItem) OnMouseDown(rx, ry int) {
	for i, item := range ti.t.items {
		if item == ti {
			ti.t.SetActive(i)
			return
		}
	}
}

func (ti *TabItem) MinSize() (int, int) {
	return len(ti.label), 1
}

func (ti *TabItem) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: ti,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

/*
// how to make label for tabBar
	format := " %s x "
	rectWidth := 17
	labelWidth := rectWidth - 4
	label := "untitled"
	if runewidth.StringWidth(label) <= labelWidth {
		label = runewidth.FillRight(label, labelWidth)
	} else {
		label = runewidth.Truncate(label, labelWidth, "…")
	}
	out := fmt.Sprintf(format, label)
*/

func (ti *TabItem) Render(s Screen, rect Rect, style Style) {
	st := style.Apply()
	if ti.hovered || ti == ti.t.items[ti.t.active] {
		st = st.Underline(true).Bold(true).Italic(true)
	}
	drawString(s, rect.X, rect.Y, rect.W, ti.label, st)
}

func (ti *TabItem) FocusTarget() Element {
	return ti.body
}

func (ti *TabItem) OnFocus() {}
func (ti *TabItem) OnBlur()  {}

type TabView struct {
	items  []*TabItem
	active int
}

func (t *TabView) Append(label string, e Element) *TabView {
	t.items = append(t.items, &TabItem{
		t:     t,
		label: label,
		body:  e,
	})
	return t
}

func (t *TabView) SetActive(i int) {
	if i >= 0 && i < len(t.items) {
		t.active = i
	}
}

func (t *TabView) MinSize() (int, int) {
	maxW, maxH := 0, 0
	for _, item := range t.items {
		w, h := item.body.MinSize()
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

	hs := HStack().Spacing(1)
	for i, item := range t.items {
		hs.Append(item)
		if i != len(t.items)-1 {
			hs.Append(Divider())
		}
	}
	n.Children = append(n.Children, hs.Layout(x, y, w, 1))

	if t.active >= 0 && t.active < len(t.items) {
		node := t.items[t.active].body.Layout(x, y+1, w, h-1)
		n.Children = append(n.Children, node)
	}
	return n
}

func (t *TabView) Render(s Screen, rect Rect, style Style) {
	// do nothing, children render themselves
}

func (t *TabView) FocusTarget() Element {
	if t.active < 0 || t.active >= len(t.items) {
		return t
	}
	return t.items[t.active]
}

func (t *TabView) OnFocus() {}
func (t *TabView) OnBlur()  {}

func (t *TabView) Grow(weight ...int) *grower {
	return Grow(t, weight...)
}

// Frame forces child to fill the frame.
// w and h can be 0, means min size.
func (t *TabView) Frame(w, h int) *Frame {
	return &Frame{W: w, H: h, Child: t}
}

// ---------------------------------------------------------------------
// APP RUNNER
// ---------------------------------------------------------------------

type App struct {
	Screen  Screen
	Root    Element
	focused Element
	hover   Element
	tree    *LayoutNode // layout tree
	done    chan struct{}
	QuitKey tcell.Key // key to quit the app, default is Escape

	clickPoint Point
}

func NewApp(root Element) *App {
	return &App{
		Root:    root,
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

// Render builds and render the layout tree
func (a *App) Render() {
	w, h := a.Screen.Size()
	a.tree = a.Root.Layout(0, 0, w, h)
	drawTree(a.tree, a.Screen, DefaultStyle)
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

	for _, child := range n.Children {
		if e, local := hitTest(child, p); e != nil {
			return e, local
		}
	}

	return n.Element, Point{
		X: p.X - n.Rect.X,
		Y: p.Y - n.Rect.Y,
	}
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
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	a.Screen = screen

	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.EnableMouse()

	draw := func() {
		screen.Clear()
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
		case *EventResize:
			draw()
			screen.Sync()
		case *EventKey:
			a.handleKey(ev)
		case *EventMouse:
			a.handleMouse(ev)
		}
	}
}

func (a *App) handleKey(ev *tcell.EventKey) {
	log.Printf("key: %s", ev.Name())
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
	hit, local := hitTest(a.tree, Point{X: x, Y: y})
	if hit == nil {
		return
	}
	lx, ly := local.X, local.Y

	a.updateHover(hit, lx, ly)

	switch ev.Buttons() {
	case tcell.ButtonPrimary:
		a.clickPoint = Point{X: x, Y: y}
		// mouse down
		if i, ok := hit.(Clickable); ok {
			i.OnMouseDown(lx, ly)
		}

		// shift focus
		a.Focus(hit)
		if _, ok := a.focused.(KeyHandler); !ok {
			a.Screen.HideCursor()
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
