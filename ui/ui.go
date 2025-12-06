// Package ui provides a lightweight text-based user interface toolkit built on top of tcell.
// It offers a clean event–state–render pipeline with basic UI components and layouts.
package ui

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

func init() {
	SetLightTheme()
}

var (
	colorFG     string
	colorBG     string
	colorCursor = "#D88A40"
	colorBorder = "#D0D0D0"

	StyleHover    Style
	StyleSelected Style

	// light:
	// #FAFAFB background, or #F7F7F8
	// #F2F2F4 hover
	// #E4E6EA selected
	// #D0D0D0 border, divider
	// #D88A40 cursor

	// dark:
	// #1E1E20 background
	// #2A2A2E hover
	// #DCEAF7 selected
)

func SetLightTheme() {
	colorFG = "black"
	colorBG = "#FAFAFB"
	StyleHover = Style{Background: "#F2F2F4"}
	StyleSelected = Style{Background: "#E4E6EA", Foreground: "black"}
}

func SetDarkTheme() {
	colorFG = "white"
	colorBG = "#1E1E20"
	StyleHover = Style{Background: "#2A2A2E"}
	StyleSelected = Style{Background: "#DCEAF7", Foreground: "black"}
}

type Screen = tcell.Screen

// Element is the interface implemented by all UI elements.
type Element interface {
	MinSize() (w, h int)
	// Layout computes the layout node for this element given the position and size.
	Layout(x, y, w, h int) *LayoutNode
	// Render draws the element onto the screen within the given rectangle.
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
	Foreground string
	Background string
	Bold       bool
	Italic     bool
	Underline  bool
}

// Apply convert Style to tcell type, uses current color theme by default.
func (s Style) Apply() tcell.Style {
	st := tcell.StyleDefault
	if s.Foreground != "" {
		st = st.Foreground(tcell.GetColor(s.Foreground))
	} else {
		st = st.Foreground(tcell.GetColor(colorFG))
	}
	if s.Background != "" {
		st = st.Background(tcell.GetColor(s.Background))
	} else {
		st = st.Background(tcell.GetColor(colorBG))
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
	return st
}

func (s Style) Merge(parent Style) Style {
	ns := s
	if ns.Background == "" {
		ns.Background = parent.Background
	}
	if ns.Foreground == "" {
		ns.Foreground = parent.Foreground
	}

	ns.Bold = ns.Bold || parent.Bold
	ns.Italic = ns.Italic || parent.Italic
	ns.Underline = ns.Underline || parent.Underline
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
	// fill the remain space with the same background color
	// for i := offset; i < w; i++ {
	// 	s.SetContent(x+i, y, ' ', nil, style)
	// }
}

// ---------------------------------------------------------------------
// components
// ---------------------------------------------------------------------

type Text struct {
	Style
	Label string
}

func NewText(c string) *Text { return &Text{Label: c} }

func (t *Text) Bold() *Text      { t.Style.Bold = true; return t }
func (t *Text) Italic() *Text    { t.Style.Italic = true; return t }
func (t *Text) Underline() *Text { t.Style.Underline = true; return t }
func (t *Text) Foreground(color string) *Text {
	t.Style.Foreground = color
	return t
}
func (t *Text) Background(color string) *Text {
	t.Style.Background = color
	return t
}

func (t *Text) MinSize() (int, int) { return len(t.Label), 1 }
func (t *Text) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (t *Text) Render(s Screen, rect Rect) {
	DrawString(s, rect.X, rect.Y, rect.W, t.Label, t.Style.Apply())
}

type Button struct {
	Style
	Label   string
	OnClick func()
	hovered bool
	pressed bool
}

// NewButton creates a new button element with the given label.
func NewButton(label string) *Button {
	return &Button{Label: label}
}

func (b *Button) MinSize() (int, int) { return runewidth.StringWidth(b.Label) + 2, 1 }
func (b *Button) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: b,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (b *Button) Render(s Screen, rect Rect) {
	st := b.Style
	if b.hovered {
		st = st.Merge(StyleHover)
	}
	if b.pressed {
		st.Background = "gray"
	}
	label := " " + b.Label + " "
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

// TextInput is a single-line editable text input field.
type TextInput struct {
	text     []rune
	cursor   int
	focused  bool
	style    Style
	onChange func()
}

// func NewTextInput(placeholder string) *TextInput {
// 	return &TextInput{
// 		style: DefaultStyle,
// 	}
// }

func (t *TextInput) Text() string {
	return string(t.text)
}

func (t *TextInput) SetText(s string) {
	t.text = []rune(s)
	t.cursor = len(t.text)
}

func (t *TextInput) OnChange(fn func()) *TextInput {
	t.onChange = fn
	return t
}

func (t *TextInput) Foreground(c string) *TextInput {
	t.style.Foreground = c
	return t
}
func (t *TextInput) Background(c string) *TextInput {
	t.style.Background = c
	return t
}

func (t *TextInput) MinSize() (int, int) { return 10, 1 }

func (t *TextInput) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (t *TextInput) Render(s Screen, rect Rect) {
	st := t.style.Apply()
	text := runewidth.FillRight(string(t.text), rect.W)
	DrawString(s, rect.X, rect.Y, rect.W, text, st)
	if t.focused && t.cursor < rect.W {
		s.ShowCursor(rect.X+t.cursor, rect.Y)
	}
}

func (t *TextInput) FocusTarget() Element { return t }
func (t *TextInput) OnFocus()             { t.focused = true }
func (t *TextInput) OnBlur()              { t.focused = false }

func (t *TextInput) HandleKey(ev *tcell.EventKey) {
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
				t.onChange()
			}
		}
	case tcell.KeyDelete:
		if t.cursor < len(t.text) {
			t.text = slices.Delete(t.text, t.cursor, t.cursor+1)
			if t.onChange != nil {
				t.onChange()
			}
		}
	case tcell.KeyRune:
		r := ev.Rune()
		t.text = slices.Insert(t.text, t.cursor, r)
		t.cursor++
		if t.onChange != nil {
			t.onChange()
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
	}
}

func (t *TextEditor) Foreground(c string) *TextEditor {
	t.style.Foreground = c
	return t
}
func (t *TextEditor) Background(c string) *TextEditor {
	t.style.Background = c
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

const tabSize = 4

func tabToSpace(line []rune) (newLine []rune) {
	newLine = make([]rune, 0, len(line))
	var total int
	for _, r := range line {
		if r != '\t' {
			newLine = append(newLine, r)
			total += runewidth.RuneWidth(r)
			continue
		}

		spaces := tabSize - total%tabSize
		for range spaces {
			newLine = append(newLine, ' ')
		}
		total += spaces
	}
	return newLine
}

// convert content index to screen cursor
func indentCursor(line []rune, i int) int {
	var total int
	for j, r := range line {
		if j == i {
			return total
		}
		if r != '\t' {
			total += runewidth.RuneWidth(r)
			continue
		}

		spaces := tabSize - total%tabSize
		total += spaces
	}
	return total
}

// convert clicking position to content index
func unIndentCursor(line []rune, clickX int) int {
	var total int
	for j, r := range line {
		if total >= clickX {
			if (total - clickX) > tabSize/2 {
				// optional, better location
				return j - 1
			}
			return j
		}
		if r != '\t' {
			total += runewidth.RuneWidth(r)
			continue
		}

		spaces := tabSize - total%tabSize
		total += spaces
	}
	if total >= clickX {
		return len(line) - 1
	}
	// Handle a click past the end of the line
	return len(line)
}

func (t *TextEditor) Render(s Screen, rect Rect) {
	st := t.style.Apply()
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

		line := tabToSpace(t.content[row])
		if row == t.row {
			cursorFound = true
			// log.Printf("line: %s, t.col: %d, %v", string(t.content[row]),
			// 	t.col, indentCursor(t.content[row], t.col))
			cursorX = contentX + indentCursor(t.content[row], t.col)
			cursorY = rect.Y + i
		}

		// Render Line Number (UNCONDITIONAL)
		lineNum := row + 1
		numStr := fmt.Sprintf("%*d ", lineNumWidth-1, lineNum)
		DrawString(s, rect.X, rect.Y+i, lineNumWidth, numStr, lineNumStyle)

		// Render Line Content
		DrawString(s, contentX, rect.Y+i, contentW, string(line), st)
	}

	// Place the cursor
	if t.focused && cursorFound {
		s.ShowCursor(cursorX, cursorY)
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
	case tcell.KeyTAB:
		t.content[t.row] = slices.Insert(currentLine, t.col, '\t')
		t.col++
	}

	if t.onChange != nil {
		t.onChange()
	}
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
	targetCol := unIndentCursor(currentLine, clickedX)

	t.col = targetCol
	t.adjustCol()
	if t.onChange != nil {
		t.onChange()
	}
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

type ListView struct {
	Items    []ListItem
	Hovered  int // -1 means nothing hovered
	Selected int // -1 means nothing selected, changes only on click
}

type ListItem struct {
	Label  string
	Action func()
}

func NewListView() *ListView {
	return &ListView{
		Hovered:  -1,
		Selected: -1,
	}
}

func (l *ListView) Append(text string, handler func()) {
	l.Items = append(l.Items, ListItem{Label: text, Action: handler})
}

func (l *ListView) Clear() {
	l.Items = nil
	l.Hovered = -1
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
			st = StyleSelected
		case l.Hovered:
			st = StyleHover
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
		l.Hovered = -1
		return
	}
	l.Hovered = ry
}

func (l *ListView) OnMouseLeave() { l.Hovered = -1 }

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
	return runewidth.StringWidth(ti.label), 1
}

func (ti *TabItem) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: ti,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

var StyleActiveTab = Style{Underline: true}

func (ti *TabItem) Render(s Screen, rect Rect) {
	var st Style
	if ti == ti.t.items[ti.t.active] {
		st = StyleActiveTab
	} else if ti.hovered {
		st = StyleHover
	}
	DrawString(s, rect.X, rect.Y, rect.W, ti.label, st.Apply())
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

func (t *TabView) Render(s Screen, rect Rect) {
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
	style := Style{Foreground: colorBorder}
	if !d.vertical {
		for i := range rect.W {
			s.SetContent(rect.X+i, rect.Y+rect.H-1, hChar, nil, style.Apply())
		}
	} else {
		for i := range rect.H {
			s.SetContent(rect.X+rect.W-1, rect.Y+i, vChar, nil, style.Apply())
		}
	}
}

type Empty struct{}

func (e Empty) MinSize() (int, int)               { return 0, 0 }
func (e Empty) Layout(x, y, w, h int) *LayoutNode { return nil }
func (e Empty) Render(Screen, Rect)               {}

// Spacer fills the remaining space between siblings inside an HStack or VStack.
var Spacer = Grow(Empty{})

// ---------------------------------------------------------------------
// Containers
// ---------------------------------------------------------------------

// vstack is a vertical layout container.
// Itself does not apply any visual styling like background colors, borders,
// it is completely transparent and invisible
type vstack struct {
	children []Element
	spacing  int
}

// VStack arranges children vertically.
func VStack(children ...Element) *vstack {
	return &vstack{children: children}
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
			ch = min(int(math.Ceil(float64(g.weight)*share)), remain)
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

func (v *vstack) Render(s Screen, rect Rect) {
	// no-op
}

func (v *vstack) Append(e ...Element) *vstack {
	v.children = append(v.children, e...)
	return v
}

// Spacing sets the spacing (in rows) between child elements.
func (v *vstack) Spacing(p int) *vstack {
	v.spacing = p
	return v
}

// scale the container
func (v *vstack) Grow(weight ...int) *grower { return Grow(v, weight...) }

// add border
func (v *vstack) Border(color string) *Box {
	return NewBox(v).Foreground(color)
}

// hstack is a horizontal layout container.
// Itself does not apply any visual styling like background colors, borders,
// it is completely transparent and invisible
type hstack struct {
	children []Element
	spacing  int
}

// HStack arranges children horizontally.
func HStack(children ...Element) *hstack {
	return &hstack{children: children}
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
			cw = min(int(math.Ceil(float64(g.weight)*share)), remain)
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

func (hs *hstack) Render(s Screen, rect Rect) {
	// no-op
}

func (hs *hstack) Append(e ...Element) *hstack {
	hs.children = append(hs.children, e...)
	return hs
}

// Spacing sets the spacing (in columns) between child elements.
func (hs *hstack) Spacing(p int) *hstack { hs.spacing = p; return hs }

func (hs *hstack) Grow(weight ...int) *grower {
	return Grow(hs, weight...)
}

func (hs *hstack) Border(color string) *Box {
	return NewBox(hs).Foreground(color)
}

// grower is a element wrapper, won't render
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

func (g *grower) Render(s Screen, rect Rect) {
	// do nothing
}

type padding struct {
	child                    Element
	top, right, bottom, left int
}

// Padding adds padding around its child.
func Padding(child Element, p int) *padding {
	return &padding{
		child:  child,
		top:    p,
		right:  p,
		bottom: p,
		left:   p,
	}
}

// PaddingH adds horizontal padding around its child.
func PaddingH(child Element, p int) *padding {
	return &padding{
		child: child,
		right: p,
		left:  p,
	}
}

// PaddingV adds vertical padding around its child.
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

func (p *padding) Render(s Screen, rect Rect) {
	// no-op
}

// Box draws border around the child.
type Box struct {
	Style
	child Element
}

func NewBox(child Element) *Box {
	return &Box{child: child}
}

func (b *Box) MinSize() (w, h int) {
	cw, ch := b.child.MinSize()
	return cw + 2, ch + 2
}
func (b *Box) Layout(x, y, w, h int) *LayoutNode {
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

// Box Drawing charaters
const (
	hChar = '─'
	vChar = '│'
	cTL   = '┌'
	cTR   = '┐'
	cBL   = '└'
	cBR   = '┘'
)

func (b *Box) Render(s Screen, rect Rect) {
	// Too small to draw a border
	if rect.W < 2 || rect.H < 2 {
		return
	}

	style := Style{Foreground: colorBorder}
	st := b.Style.Merge(style).Apply()
	// Top and bottom borders
	for i := 0; i < rect.W; i++ {
		s.SetContent(rect.X+i, rect.Y, hChar, nil, st)
		s.SetContent(rect.X+i, rect.Y+rect.H-1, hChar, nil, st)
	}
	// Left and right borders
	for i := 0; i < rect.H; i++ {
		s.SetContent(rect.X, rect.Y+i, vChar, nil, st)
		s.SetContent(rect.X+rect.W-1, rect.Y+i, vChar, nil, st)
	}
	// Corners
	s.SetContent(rect.X, rect.Y, cTL, nil, st)
	s.SetContent(rect.X+rect.W-1, rect.Y, cTR, nil, st)
	s.SetContent(rect.X, rect.Y+rect.H-1, cBL, nil, st)
	s.SetContent(rect.X+rect.W-1, rect.Y+rect.H-1, cBR, nil, st)
}

func (b *Box) Foreground(color string) *Box {
	b.Style.Foreground = color
	return b
}

func (b *Box) Background(color string) *Box {
	b.Style.Background = color
	return b
}

// Frame is a fixed-size wrapper that always reports the size given by W and H.
// It layouts exactly one child element and forces the child to fill the frame.
type Frame struct {
	Style     // TODO: maybe embed?
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

// ResetRect resets the content of the given rectangle to the specified style.
func ResetRect(s Screen, rect Rect, style Style) {
	st := style.Apply()
	for x := rect.X; x < rect.X+rect.W; x++ {
		for y := rect.Y; y < rect.Y+rect.H; y++ {
			// when debugging, printing '.' would be better
			s.SetContent(x, y, ' ', nil, st)
		}
	}
}

func (f *Frame) Render(s Screen, rect Rect) {
	ResetRect(s, rect, f.Style)
}

// Overlay represents a floating element displayed on top of the main UI.
// TODO: maybe add border? dismiss on click outside?
type Overlay struct {
	Rect  Rect
	Child Element
}

func (o *Overlay) MinSize() (w, h int) { return o.Rect.W, o.Rect.H }

func (o *Overlay) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element:  o,
		Rect:     Rect{X: x, Y: y, W: w, H: h},
		Children: []*LayoutNode{o.Child.Layout(x, y, w, h)},
	}
}

func (o *Overlay) Render(s Screen, rect Rect) {
	// no-op
}

// ---------------------------------------------------------------------
// APP RUNNER
// ---------------------------------------------------------------------

type App struct {
	screen  Screen
	root    Element
	focused Element
	hover   Element
	tree    *LayoutNode
	done    chan struct{}

	clickPoint Point
	keymap     map[string]func()
	overlay    *Overlay
}

var app *App

func Default() *App {
	if app == nil {
		app = NewApp(VStack())
	}
	return app
}

func NewApp(root Element) *App {
	a := &App{
		root:   root,
		done:   make(chan struct{}),
		keymap: make(map[string]func()),
	}
	a.keymap["Ctrl+C"] = a.Stop
	return a
}

func (a *App) SetRoot(e Element) {
	a.root = e
}
func (a *App) Root() Element {
	return a.root
}

func (a *App) Screen() Screen {
	return a.screen
}

func (a *App) BindKey(key string, action func()) {
	a.keymap[key] = action
}

func (a *App) SetOverlay(e Element, rect Rect) {
	a.overlay = &Overlay{
		Rect:  rect,
		Child: e,
	}
}

func (a *App) ClearOverlay() {
	a.overlay = nil
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
		node := o.Layout(o.Rect.X, o.Rect.Y, o.Rect.W, o.Rect.H)
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

func (a *App) Focus(e Element) {
	// log.Print("try focus: ", prettyType(e))
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
	if _, ok := a.focused.(KeyHandler); !ok {
		a.screen.HideCursor()
	}
	log.Print("focused: ", prettyType(a.focused))
}

func prettyType(v any) string {
	t := reflect.TypeOf(v)
	if t == nil {
		return "<nil>"
	}
	return t.String()
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

// Run starts the main event loop.
func (a *App) Run() error {
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
	screen.SetCursorStyle(tcell.CursorStyleDefault, tcell.GetColor(colorCursor))

	draw := func() {
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

func (a *App) handleKey(ev *tcell.EventKey) {
	log.Printf("key: %s", ev.Name())
	if f, ok := a.keymap[ev.Name()]; ok {
		f()
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
