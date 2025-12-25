// Package ui provides a lightweight text-based user interface toolkit built on top of tcell.
// It offers a clean event–state–render pipeline with basic UI components and layouts.
package ui

import (
	"fmt"
	"go/token"
	"log"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

type Screen = tcell.Screen

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
	// FocusTarget determines whom should actually receive focus.
	// Can be used to retain or delegate focus.
	FocusTarget() Element
	// OnFocus is called when the element receives focus.
	OnFocus()
	// OnBlur is called when the element loses focus.
	OnBlur()
	HandleKey(ev *tcell.EventKey)
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
	Style
	Label string
}

func NewText(c string) *Text {
	t := &Text{Label: c}
	return t
}

func (t *Text) Bold() *Text      { t.Style.FontBold = true; return t }
func (t *Text) Italic() *Text    { t.Style.FontItalic = true; return t }
func (t *Text) Underline() *Text { t.Style.FontUnderline = true; return t }
func (t *Text) Foreground(color string) *Text {
	t.Style.FG = color
	return t
}
func (t *Text) Background(color string) *Text {
	t.Style.BG = color
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
	label   string
	onClick func()
	hovered bool
	pressed bool
}

// NewButton creates a new button element with the given label.
func NewButton(label string, onClick func()) *Button {
	b := &Button{label: label, onClick: onClick}
	return b
}

func (b *Button) MinSize() (int, int) { return runewidth.StringWidth(b.label) + 2, 1 }
func (b *Button) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: b,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}
func (b *Button) Render(s Screen, rect Rect) {
	st := b.Style
	if b.pressed {
		st.BG = Theme.Selection
	}
	label := " " + b.label + " "
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
		if b.onClick != nil {
			b.onClick()
		}
	}
	b.pressed = false
}

func (b *Button) OnClick() {
	b.onClick()
}

func (b *Button) Foreground(color string) *Button {
	b.Style.FG = color
	return b
}

func (b *Button) Background(color string) *Button {
	b.Style.BG = color
	return b
}

// TextInput is a single-line editable text input field.
type TextInput struct {
	text        []rune
	cursor      int
	focused     bool
	style       Style
	onChange    func()
	placeHolder string
	selStart    int  // 選取起點 (rune index)
	pressed     bool // 標記滑鼠是否按下以進行拖拽
}

func NewTextInput() *TextInput {
	t := &TextInput{}
	return t
}

func (t *TextInput) Text() string {
	return string(t.text)
}

func (t *TextInput) SetText(s string) {
	t.text = []rune(s)
	t.cursor = len(t.text)
	t.selStart = t.cursor
	if t.onChange != nil {
		t.onChange()
	}
}

func (t *TextInput) OnChange(fn func()) *TextInput {
	t.onChange = fn
	return t
}

func (t *TextInput) SetPlaceholder(s string) {
	t.placeHolder = s
}

func (t *TextInput) Foreground(c string) *TextInput {
	t.style.FG = c
	return t
}
func (t *TextInput) Background(c string) *TextInput {
	t.style.BG = c
	return t
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
	// log.Print(start, end, hasSel)
	baseStyle := t.style.Apply()
	selStyle := t.style.Merge(Style{BG: Theme.Selection}).Apply()

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

func (t *TextInput) FocusTarget() Element { return t }
func (t *TextInput) OnFocus()             { t.focused = true }
func (t *TextInput) OnBlur()              { t.focused = false }

func (t *TextInput) HandleKey(ev *tcell.EventKey) {
	if !t.focused {
		return
	}

	// 簡單邏輯：按下任何非 Shift 的移動鍵就重置選取起點
	resetSelection := true

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
	}

	if resetSelection && !t.pressed {
		t.selStart = t.cursor
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
	if !t.pressed {
		t.pressed = true
		t.selStart = x
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
	t.selStart = start
	t.cursor = end
}

// 取得正規化後的選取範圍 (start <= end)
func (t *TextInput) selection() (int, int, bool) {
	if t.selStart == t.cursor {
		return 0, 0, false
	}
	start, end := t.selStart, t.cursor
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

// OnChange register a callback function that will be called on content changed
func (tv *TextViewer) OnChange(f func()) { tv.onChange = f }

// TextEditor is a multi-line editable text area.
type TextEditor struct {
	content  [][]rune // simple 2D slice of runes, avoid over-engineering
	row, col int      // 既是游標，也是選取的「動端」(Head)
	offsetY  int      // Vertical scroll offset
	focused  bool
	style    Style
	viewH    int // last rendered height
	contentX int
	onChange func()
	Dirty    bool

	anchorRow int  // 選取的「靜端」(Anchor) 起點行
	anchorCol int  // 選取的「靜端」(Anchor) 起點列
	selecting bool // 是否處於選取狀態
	pressed   bool // mouse pressed

	// desired visual column for cursor alignment during vertical (up/down) navigation
	desiredVisualCol int
}

func NewTextEditor() *TextEditor {
	t := &TextEditor{
		content: [][]rune{{}}, // Start with one empty line of runes
	}
	return t
}

func (t *TextEditor) Foreground(c string) *TextEditor {
	t.style.FG = c
	return t
}
func (t *TextEditor) Background(c string) *TextEditor {
	t.style.BG = c
	return t
}

func (t *TextEditor) Len() int {
	return len(t.content)
}

func (t *TextEditor) String() string {
	var sb strings.Builder
	for i, line := range t.content {
		sb.WriteString(string(line))
		if i != len(t.content)-1 {
			sb.WriteByte('\n')
		}
	}
	// append final newline
	if len(t.content) > 0 && len(t.content[len(t.content)-1]) != 0 {
		sb.WriteByte('\n')
	}
	return sb.String()
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

func (t *TextEditor) Cursor() (row, col int) {
	return t.row, t.col
}

// SetCursor set the content cursor to the given row(line index)
// and col(rune index in the line)
func (t *TextEditor) SetCursor(row, col int) {
	if row < 0 || row >= len(t.content) || col < 0 {
		return
	}
	t.CancelSelection()
	t.row = row
	t.col = col
	t.adjustCol()
}

// CenterRow 僅負責將特定行移動到螢幕中心，提供最大上下視野
// mainly for jumping to search result, symbol
func (t *TextEditor) CenterRow(row int) {
	if t.viewH <= 0 {
		return
	}

	t.offsetY = row - (t.viewH / 2)
	t.clampScroll() // 抽離出的邊界檢查邏輯
}

// ScrollTo adjusts offsetY to ensures the row is visible on the screen,
// does minimal scrolling, and mainly for arrow key movement and typing.
func (t *TextEditor) ScrollTo(row int) {
	if t.viewH <= 0 {
		return
	}

	const scrolloff = 1 // 預留 1 行邊距，體驗更好

	// 處理下方出界
	if t.row >= t.offsetY+t.viewH-scrolloff {
		t.offsetY = t.row - t.viewH + 1 + scrolloff
	}

	// 處理上方出界
	if t.row < t.offsetY+scrolloff {
		t.offsetY = t.row - scrolloff
	}

	t.clampScroll()
}

func (t *TextEditor) clampScroll() {
	maxOffset := max(0, len(t.content)-t.viewH)
	if t.offsetY > maxOffset {
		t.offsetY = maxOffset
	}
	if t.offsetY < 0 {
		t.offsetY = 0
	}
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

// visualColFromLine returns the visual column (in terminal cells)
// corresponding to rune index i in the line.
//
// Tabs are expanded using tabSize, and rune widths are measured with
// runewidth.RuneWidth. If i is beyond the end of the line, the total
// visual width of the entire line is returned.
func visualColFromLine(line []rune, i int) int {
	var col int
	for j, r := range line {
		if j == i {
			return col
		}

		if r == '\t' {
			col += tabSize - col%tabSize
		} else {
			col += runewidth.RuneWidth(r)
		}
	}
	return col
}

// visualColToLine converts a visual column position into rune index in the line.
//
// Tabs are expanded using tabSize. When col falls inside a tab expansion,
// the function chooses the nearest rune boundary; if the column is closer to
// the previous column than the next, it may return the preceding rune index.
//
// If col is past the end of the line, len(line) is returned.
func visualColToLine(line []rune, col int) int {
	var total int
	for i, r := range line {
		next := total
		if r == '\t' {
			next += tabSize - total%tabSize
		} else {
			next += runewidth.RuneWidth(r)
		}

		if col < next {
			return i
		}
		total = next
	}
	return len(line)
}

func (t *TextEditor) drawRune(s tcell.Screen, x, y int, maxWidth int, r rune, visualCol int, style Style) int {
	if maxWidth <= 0 {
		return 0
	}

	// TAB
	if r == '\t' {
		spaces := tabSize - visualCol%tabSize
		if spaces > maxWidth {
			spaces = maxWidth
		}
		for i := range spaces {
			s.SetContent(x+i, y, ' ', nil, style.Apply())
		}
		return spaces
	}

	// other rune
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		w = 1
	}
	if w > maxWidth {
		return 0
	}
	s.SetContent(x, y, r, nil, style.Apply())
	return w
}

func (t *TextEditor) Render(s Screen, rect Rect) {
	t.viewH = rect.H

	// Dynamic width calculation (for proper right-justification)
	numLines := len(t.content)
	if numLines == 0 {
		numLines = 1
	}
	actualNumDigits := len(strconv.Itoa(numLines))
	lineNumWidth := actualNumDigits + 2
	lineNumStyle := Style{FG: "silver"}

	contentX := rect.X + lineNumWidth + 1
	t.contentX = contentX
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
		lnStyle := lineNumStyle
		if row == t.row {
			cursorFound = true
			cursorX = contentX + visualColFromLine(line, t.col)
			cursorY = rect.Y + i
			lnStyle.BG = Theme.Selection
		}

		// draw line number
		lineNum := row + 1
		numStr := fmt.Sprintf("%*d  ", lineNumWidth-1, lineNum)
		DrawString(s, rect.X, rect.Y+i, lineNumWidth, numStr, lnStyle.Apply())

		// draw line content
		spans := highlightGo(line)
		styles := expandStyles(spans, t.style, len(line))
		visualCol := 0
		y := rect.Y + i
		for col, r := range line {
			charStyle := styles[col]
			if t.isSelected(row, col) {
				charStyle.BG = Theme.Selection
			}
			cells := t.drawRune(s, contentX+visualCol, y, contentW-visualCol, r, visualCol, charStyle)
			visualCol += cells
		}

		// draw line end indicator while selected
		if t.isSelected(row, len(line)) {
			charStyle := t.style.Merge(Style{BG: Theme.Selection})
			t.drawRune(s, contentX+visualCol, y, contentW-visualCol, ' ', visualCol, charStyle)
		}
	}

	// Place the cursor
	if t.focused {
		if cursorFound {
			s.ShowCursor(cursorX, cursorY)
		} else {
			// Cursor line is not visible, hide cursor
			s.HideCursor()
		}
	}
}
func (t *TextEditor) FocusTarget() Element { return t }
func (t *TextEditor) OnFocus()             { t.focused = true }
func (t *TextEditor) OnBlur()              { t.focused = false }

func (t *TextEditor) HandleKey(ev *tcell.EventKey) {
	currentLine := t.content[t.row]

	keepVisualCol := false
	defer func() {
		if !keepVisualCol {
			t.desiredVisualCol = 0
		}
		if t.onChange != nil {
			t.onChange()
		}
	}()

	switch ev.Key() {
	case tcell.KeyESC:
		t.CancelSelection()
	case tcell.KeyUp:
		t.CancelSelection()
		if ev.Modifiers()&tcell.ModMeta != 0 {
			t.row, t.col = 0, 0
			t.ScrollTo(t.row)
			return
		}

		keepVisualCol = true
		if t.desiredVisualCol == 0 {
			t.desiredVisualCol = visualColFromLine(currentLine, t.col)
		}
		if t.row > 0 {
			t.row--
			t.col = visualColToLine(t.content[t.row], t.desiredVisualCol)
			t.adjustCol()
			t.ScrollTo(t.row)
		}
	case tcell.KeyDown:
		t.CancelSelection()
		if ev.Modifiers()&tcell.ModMeta != 0 {
			t.row, t.col = len(t.content)-1, 0
			t.ScrollTo(t.row)
			return
		}

		keepVisualCol = true
		if t.desiredVisualCol == 0 {
			t.desiredVisualCol = visualColFromLine(currentLine, t.col)
		}
		if t.row < len(t.content)-1 {
			t.row++
			t.col = visualColToLine(t.content[t.row], t.desiredVisualCol)
			t.adjustCol()
			t.ScrollTo(t.row)
		}
	case tcell.KeyLeft:
		if ev.Modifiers()&tcell.ModMeta != 0 {
			t.CancelSelection()
			firstNonSpace := 0
			for i, ch := range t.content[t.row] {
				if !unicode.IsSpace(ch) {
					firstNonSpace = i
					break
				}
			}
			t.col = firstNonSpace
			return
		}
		if r1, c1, _, _, ok := t.Selection(); ok {
			t.row, t.col = r1, c1
			t.CancelSelection()
			return
		}

		if t.col > 0 {
			t.col--
		} else if t.row > 0 {
			t.row--
			t.col = len(t.content[t.row]) // End of previous line
			t.ScrollTo(t.row)
		}
	case tcell.KeyRight:
		if ev.Modifiers()&tcell.ModMeta != 0 {
			t.CancelSelection()
			t.col = len(t.content[t.row])
			return
		}
		if _, _, r2, c2, ok := t.Selection(); ok {
			t.row, t.col = r2, c2
			t.CancelSelection()
			return
		}
		if t.col < len(currentLine) {
			t.col++
		} else if t.row < len(t.content)-1 {
			t.row++
			t.col = 0 // Start of next line
			t.ScrollTo(t.row)
		}
	case tcell.KeyEnter:
		if r1, c1, r2, c2, ok := t.Selection(); ok {
			t.DeleteRange(r1, c1, r2, c2)
			t.CancelSelection()
		}
		head := currentLine[:t.col]
		tail := currentLine[t.col:]

		t.content[t.row] = head
		newLine := slices.Clone(tail)
		t.content = slices.Insert(t.content, t.row+1, newLine)

		t.row++
		t.col = 0
		t.ScrollTo(t.row)
		t.Dirty = true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if r1, c1, r2, c2, ok := t.Selection(); ok {
			t.DeleteRange(r1, c1, r2, c2)
			t.CancelSelection()
			return
		}
		if t.col > 0 {
			t.content[t.row] = slices.Delete(currentLine, t.col-1, t.col)
			t.col--
		} else if t.row > 0 {
			prevLine := t.content[t.row-1]
			t.col = len(prevLine)
			t.content[t.row-1] = append(prevLine, currentLine...)

			t.content = slices.Delete(t.content, t.row, t.row+1)
			t.row--
			t.ScrollTo(t.row)
		}
		t.Dirty = true
	case tcell.KeyRune:
		// 如果有選取，先刪除選取範圍，再插入字元
		if r1, c1, r2, c2, ok := t.Selection(); ok {
			t.DeleteRange(r1, c1, r2, c2)
			t.CancelSelection()
		}
		r := ev.Rune()
		t.content[t.row] = slices.Insert(currentLine, t.col, r)
		t.col++
		t.Dirty = true
	case tcell.KeyTAB:
		if r1, c1, r2, c2, ok := t.Selection(); ok {
			t.DeleteRange(r1, c1, r2, c2)
			t.CancelSelection()
		}
		t.content[t.row] = slices.Insert(currentLine, t.col, '\t')
		t.col++
		t.Dirty = true
	}

}

func (t *TextEditor) OnMouseUp(x, y int) {
	t.pressed = false
	if t.row == t.anchorRow && t.col == t.anchorCol {
		t.selecting = false
	}
}

func (t *TextEditor) OnMouseDown(x, y int) {
	// Calculate the target row (relative to content)
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
	if t.onChange != nil {
		defer t.onChange()
	}

	// Calculate the target column (rune index)
	visualCol := max(x-t.contentX, 0)
	t.col = visualColToLine(t.content[t.row], visualCol)

	if !t.pressed {
		// 點擊瞬間，錨點與游標重合
		t.anchorRow, t.anchorCol = t.row, t.col
		t.selecting = true
		t.pressed = true
	}
}

func (t *TextEditor) OnMouseEnter() {}
func (t *TextEditor) OnMouseLeave() {}
func (t *TextEditor) OnMouseMove(lx, ly int) {
	if t.pressed {
		if t.onChange != nil {
			defer t.onChange()
		}

		// Drag to select
		targetRow := ly + t.offsetY
		if targetRow < 0 {
			targetRow = 0
		} else if targetRow >= len(t.content) {
			targetRow = len(t.content) - 1
		}
		currentLine := t.content[targetRow]
		clickedX := max(lx-t.contentX, 0)
		targetCol := visualColToLine(currentLine, clickedX)

		t.row = targetRow
		t.col = targetCol
	}
}

// Select sets the selection anchor to startRow and startCol,
// sets content cursor to endRow and endCol.
func (t *TextEditor) Select(startRow, startCol, endRow, endCol int) {
	t.anchorRow, t.anchorCol = startRow, startCol
	t.row, t.col = endRow, endCol
	t.selecting = true
}

// Selection 返回 (起始行, 起始列, 結束行, 結束列, 是否有選取)
func (e *TextEditor) Selection() (r1, c1, r2, c2 int, ok bool) {
	if !e.selecting || (e.row == e.anchorRow && e.col == e.anchorCol) {
		return 0, 0, 0, 0, false
	}

	r1, c1 = e.anchorRow, e.anchorCol
	r2, c2 = e.row, e.col

	// 確保 (r1, c1) 在 (r2, c2) 之前
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, r2 = r2, r1
		c1, c2 = c2, c1
	}
	return r1, c1, r2, c2, true
}

func (e *TextEditor) CancelSelection() {
	e.selecting = false
}

func (e *TextEditor) isSelected(r, c int) bool {
	r1, c1, r2, c2, ok := e.Selection()
	if !ok {
		return false
	}

	if r < r1 || r > r2 {
		return false
	}
	if r > r1 && r < r2 {
		return true
	}
	if r1 == r2 {
		return c >= c1 && c < c2
	}
	if r == r1 {
		return c >= c1
	}
	if r == r2 {
		return c < c2
	}
	return false
}

// SelectWord 擴展當前游標到單詞邊界
func (t *TextEditor) SelectWord() {
	if len(t.content) == 0 || t.row >= len(t.content) {
		return
	}
	line := t.content[t.row]
	if len(line) == 0 {
		return
	}

	start, end := findWordBoundary(line, t.col)
	// 讓游標停在單詞末尾，方便下次搜尋從末尾開始
	t.Select(t.row, start, t.row, end)
}

// SelectLine 擴展選中到整行
func (t *TextEditor) SelectLine() {
	if len(t.content) == 0 || t.row >= len(t.content) {
		return
	}
	if t.row < len(t.content)-1 {
		// 選中整行，並將游標移至下一行開頭（模仿主流編輯器行為）
		t.Select(t.row, 0, t.row+1, 0)
	} else {
		t.Select(t.row, 0, t.row, len(t.content[t.row]))
	}
}

// FindNext 尋找下一個匹配項並更新選區
func (t *TextEditor) FindNext(query string) {
	if query == "" {
		return
	}
	qRunes := []rune(query)
	qLen := len(qRunes)
	lineCount := len(t.content)

	// 從當前位置之後開始搜尋
	startRow := t.row
	startCol := t.col

	for i := 0; i < lineCount; i++ {
		// 使用取模實現 Wrap Around (循環搜尋)
		currentRow := (startRow + i) % lineCount
		line := t.content[currentRow]

		// 如果是起始行，從當前列開始找；否則從行首開始找
		searchFromCol := 0
		if i == 0 {
			searchFromCol = startCol
		}

		if searchFromCol >= len(line) && i == 0 {
			continue
		}

		// 在當前行中尋找匹配
		foundIdx := -1
		remaining := line[searchFromCol:]
		for j := 0; j <= len(remaining)-qLen; j++ {
			match := true
			for k := 0; k < qLen; k++ {
				if remaining[j+k] != qRunes[k] {
					match = false
					break
				}
			}
			if match {
				foundIdx = j
				break
			}
		}

		if foundIdx != -1 {
			// 找到匹配項後的座標計算
			actualCol := searchFromCol + foundIdx

			// 1. 更新選區：從匹配項開始到結束
			t.anchorRow, t.anchorCol = currentRow, actualCol
			t.row = currentRow
			t.col = actualCol + qLen
			t.selecting = true

			// 3. 確保視覺調整
			t.CenterRow(currentRow)
			if t.onChange != nil {
				t.onChange()
			}
			return
		}
	}
}

// SelectedText 回傳當前選區的字串內容
func (t *TextEditor) SelectedText() string {
	sRow, sCol, eRow, eCol, ok := t.Selection()
	if !ok {
		return ""
	}

	if sRow == eRow {
		return string(t.content[sRow][sCol:eCol])
	}

	var sb strings.Builder
	sb.WriteString(string(t.content[sRow][sCol:]))
	sb.WriteByte('\n')
	for i := sRow + 1; i < eRow; i++ {
		sb.WriteString(string(t.content[i]))
		sb.WriteByte('\n')
	}
	sb.WriteString(string(t.content[eRow][:eCol]))
	return sb.String()
}

func findWordBoundary(line []rune, pos int) (start, end int) {
	isWordChar := func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
	}

	if pos >= len(line) {
		pos = len(line) - 1
	}
	if pos < 0 {
		return 0, 0
	}

	// 向左找
	start = pos
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	// 向右找
	end = pos
	if end < len(line) && isWordChar(line[end]) {
		for end < len(line) && isWordChar(line[end]) {
			end++
		}
	} else if end < len(line) {
		// 如果游標不在單詞上，至少選中當前字符
		end++
	}
	return start, end
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

// Cursor returns the current content index
func (t *TextEditor) Debug() string {
	s := fmt.Sprintf("Line %d, Column %d", t.row+1, t.col+1)
	if startRow, startCol, endRow, endCol, ok := t.Selection(); ok {
		s += fmt.Sprintf(", Selecting from (%d, %d) to (%d, %d)",
			startRow+1, startCol+1, endRow+1, endCol+1)
	}
	return s
}

// OnChange sets a callback function that is called whenever the text content changes.
func (t *TextEditor) OnChange(fn func()) {
	t.onChange = fn
}

// Line return a line of text on the given row index.
func (t *TextEditor) Line(i int) []rune {
	return slices.Clone(t.content[i])
}

// InsertText simulates a paste operation: it inserts a string 's' at the current
// cursor position (t.row, t.col), correctly handling any embedded newlines ('\n').
func (t *TextEditor) InsertText(s string) {
	if s == "" {
		return
	}
	// 如果有選取，先刪除選取範圍
	if r1, c1, r2, c2, ok := t.Selection(); ok {
		t.DeleteRange(r1, c1, r2, c2)
		t.CancelSelection()
	}

	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return
	}

	newContent := make([][]rune, len(lines))
	for i, line := range lines {
		newContent[i] = []rune(line)
	}

	if t.row >= len(t.content) || t.row < 0 {
		t.row = len(t.content) - 1
		if t.row < 0 {
			t.content = [][]rune{{}}
			t.row = 0
			t.col = 0
		}
	}

	currentLine := t.content[t.row]
	cursorCol := min(t.col, len(currentLine))

	head := currentLine[:cursorCol]
	tail := slices.Clone(currentLine[cursorCol:])

	if len(newContent) == 1 {
		// Single-line insert
		t.content[t.row] = append(append(head, newContent[0]...), tail...)
		t.col += len(newContent[0])
	} else {
		// Multi-line insert
		firstLine := newContent[0]
		t.content[t.row] = append(head, firstLine...)

		lastLineIndex := len(newContent) - 1
		newContent[lastLineIndex] = append(newContent[lastLineIndex], tail...)

		middleAndLastLines := newContent[1:]
		t.content = slices.Insert(t.content, t.row+1, middleAndLastLines...)

		t.row += len(newContent) - 1
		t.col = len(t.content[t.row]) - len(tail)
	}

	t.ScrollTo(t.row)
	t.Dirty = true
	if t.onChange != nil {
		t.onChange()
	}
}

// DeleteRange deletes a range of text defined by two cursor positions (start, end).
// the positions are inclusive of start and exclusive of end.
func (t *TextEditor) DeleteRange(startRow, startCol, endRow, endCol int) {
	// 1. Normalize and clamp the selection range
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, endRow = endRow, startRow
		startCol, endCol = endCol, startCol
	}

	if startRow < 0 {
		startRow = 0
	}
	if endRow >= len(t.content) {
		endRow = len(t.content) - 1
	}
	if startRow > endRow {
		return
	}

	startLine := t.content[startRow]
	endLine := t.content[endRow]

	startCol = min(startCol, len(startLine))
	endCol = min(endCol, len(endLine))

	// 2. Extract Head and Tail
	head := startLine[:startCol]
	tail := endLine[endCol:]

	// 3. Perform Merging and Deletion
	mergedLine := append(head, tail...)

	// a) Replace the starting line with the merged content
	t.content[startRow] = mergedLine

	// b) Delete intermediate lines
	if startRow < endRow {
		t.content = slices.Delete(t.content, startRow+1, endRow+1)
	}

	if len(t.content) == 0 {
		t.content = [][]rune{{}}
	}

	// 4. Update Cursor State
	t.row = startRow
	t.col = len(head)

	t.adjustCol()
	t.ScrollTo(t.row)
	t.Dirty = true
	if t.onChange != nil {
		t.onChange()
	}
}

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
	l := &ListView{
		Hovered:  -1,
		Selected: -1,
	}
	return l
}

func (l *ListView) Append(text string, action func()) {
	l.Items = append(l.Items, ListItem{Label: text, Action: action})
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
			st.BG = Theme.Selection
		case l.Hovered:
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

// var StyleActiveTab = Style{Underline: true}

func (ti *TabItem) Render(s Screen, rect Rect) {
	var st Style
	if ti == ti.t.items[ti.t.active] {
		st.FontUnderline = true
	} else if ti.hovered {
		st.BG = Theme.Hover
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

func NewTabView() *TabView {
	t := &TabView{}
	return t
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

// Spacer fills the remaining space between siblings inside an HStack or VStack.
var Spacer = Grow(Empty{})

// ---------------------------------------------------------------------
// Containers
// ---------------------------------------------------------------------

// layoutSpec is a decorator.
// It can acts like marker, tells the layout algorithm the grow a element.
// It can acts like wrapper, wraps element and changes it's rendering (padding, border, frame).
type layoutSpec struct {
	Element
	grow                   int // the weight to grow
	padT, padB, padL, padR int
	width, height          int // Frame 約束
	border                 bool
}

// 實作 MinSize: 包裝容器必須把額外的空間算進去
func (s layoutSpec) MinSize() (w, h int) {
	mw, mh := s.Element.MinSize()

	// 加上 Padding
	mw += s.padL + s.padR
	mh += s.padT + s.padB

	// 加上 Border 空間 (上下左右各 1)
	if s.border {
		mw += 2
		mh += 2
	}

	// 如果有 Frame 強制約束
	if s.width > 0 && mw < s.width {
		mw = s.width
	}
	if s.height > 0 && mh < s.height {
		mh = s.height
	}

	return mw, mh
}

// 實作 Layout: 決定子組件實際拿到的矩形
func (s layoutSpec) Layout(x, y, w, h int) *LayoutNode {
	ix, iy, iw, ih := x, y, w, h

	// 1. 處理 Border 縮減
	if s.border {
		ix, iy, iw, ih = ix+1, iy+1, iw-2, ih-2
	}

	// 2. 處理 Padding 縮減
	ix += s.padL
	iy += s.padT
	iw -= (s.padL + s.padR)
	ih -= (s.padT + s.padB)

	// 3. 處理 Frame 約束 (如果給定的 w/h 超過 Frame，這裡可以做對齊處理，暫時簡化)
	if s.width > 0 && iw > s.width {
		iw = s.width
	}
	if s.height > 0 && ih > s.height {
		ih = s.height
	}

	node := NewLayoutNode(s, x, y, w, h)
	node.Children = []*LayoutNode{s.Element.Layout(ix, iy, iw, ih)}
	return node
}

// 實作 Render: 繪製裝飾（Border）
func (s layoutSpec) Render(screen Screen, rect Rect) {
	if s.border {
		// 這裡實作畫框邏輯，可以使用你的 Theme 顏色
		drawBorder(screen, rect)
	}
	// 這裡不需要處理 Padding 的 Render，因為 Layout 已經把子組件限縮在裡面了
}

// 輔助函數：取得或建立 spec
func getSpec(e Element) layoutSpec {
	if s, ok := e.(layoutSpec); ok {
		return s
	}
	return layoutSpec{Element: e}
}

// --- Functional Options ---

// Pad is a wrapper that adds spaces around the element
func Pad(e Element, amount int) Element {
	// Current implementation merges layoutSpec,
	// does not distint inner/outer padding.
	// If needed, allow nesting layoutSpec.
	s := getSpec(e)
	s.padT, s.padB, s.padL, s.padR = amount, amount, amount, amount
	return s
}

func PadH(e Element, amount int) Element {
	s := getSpec(e)
	s.padL, s.padR = amount, amount
	return s
}

func PadV(e Element, amount int) Element {
	s := getSpec(e)
	s.padT, s.padB = amount, amount
	return s
}

func Grow(e Element) Element {
	s := getSpec(e)
	s.grow = 1
	return s
}

func Frame(e Element, w, h int) Element {
	s := getSpec(e)
	s.width, s.height = w, h
	return s
}

func Border(e Element) Element {
	s := getSpec(e)
	s.border = true
	return s
}

// vstack is a vertical layout container.
// Itself does not apply any visual styling like background colors, borders,
// it is completely transparent and invisible
type vstack struct {
	children []Element
	spacing  int
}

// VStack arranges children vertically.
func VStack(children ...Element) *vstack {
	v := &vstack{children: children}
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
	n := NewLayoutNode(v, x, y, w, h)
	// First pass: measure children
	totalH := 0
	totalGrow := 0
	for _, child := range v.children {
		if i, ok := child.(layoutSpec); ok && i.grow > 0 {
			totalGrow += i.grow
		} else {
			_, ch := child.MinSize()
			totalH += ch
		}
	}

	// Compute spare space
	spare := max(h-totalH-v.spacing*(len(v.children)-1), 0)
	var share float64
	if totalGrow > 0 {
		share = float64(spare) / float64(totalGrow)
	}

	// Second pass: layout children
	used := 0
	for i, child := range v.children {
		if d, ok := child.(*divider); ok {
			d.vertical = false
		}
		_, ch := child.MinSize()
		if s, ok := child.(layoutSpec); ok && s.grow > 0 {
			expand := int(math.Ceil(float64(s.grow) * share))
			if expand > spare {
				expand = spare
			}
			ch = expand
			spare -= expand
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

// hstack is a horizontal layout container.
// Itself does not apply any visual styling like background colors, borders,
// it is completely transparent and invisible
type hstack struct {
	children []Element
	spacing  int
}

// HStack arranges children horizontally.
func HStack(children ...Element) *hstack {
	h := &hstack{children: children}
	return h
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
	n := NewLayoutNode(hs, x, y, w, h)
	// First pass: measure children
	totalWidth := 0
	totalGrow := 0
	for _, child := range hs.children {
		if s, ok := child.(layoutSpec); ok && s.grow > 0 {
			totalGrow += s.grow
		} else {
			cw, _ := child.MinSize()
			totalWidth += cw
		}
	}

	// Compute remaining space
	remain := max(w-totalWidth-hs.spacing*(len(hs.children)-1), 0)
	var share float64
	if totalGrow > 0 {
		share = float64(remain) / float64(totalGrow)
	}

	// Second pass: layout children
	used := 0
	for i, child := range hs.children {
		if div, ok := child.(*divider); ok {
			div.vertical = true
		}
		cw, _ := child.MinSize()
		if s, ok := child.(layoutSpec); ok && s.grow > 0 {
			expand := min(int(math.Ceil(float64(s.grow)*share)), remain)
			cw = expand
			remain -= expand
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

// Box Drawing charaters
const (
	hLine = '─'
	vLine = '│'
	// cornerTopLeft, cornerTopRight = '╭', '╮'
	// cornerBotLeft, cornerBotRight = '╰', '╯'
	cornerTopLeft, cornerTopRight = '┌', '┐'
	cornerBotLeft, cornerBotRight = '└', '┘'
)

func drawBorder(s Screen, rect Rect) {
	// Too small to draw a border
	if rect.W < 2 || rect.H < 2 {
		return
	}

	st := Style{FG: Theme.Border}.Apply()
	// Top and bottom borders
	for i := range rect.W {
		s.SetContent(rect.X+i, rect.Y, hLine, nil, st)
		s.SetContent(rect.X+i, rect.Y+rect.H-1, hLine, nil, st)
	}
	// Left and right borders
	for i := range rect.H {
		s.SetContent(rect.X, rect.Y+i, vLine, nil, st)
		s.SetContent(rect.X+rect.W-1, rect.Y+i, vLine, nil, st)
	}
	// Corners
	s.SetContent(rect.X, rect.Y, cornerTopLeft, nil, st)
	s.SetContent(rect.X+rect.W-1, rect.Y, cornerTopRight, nil, st)
	s.SetContent(rect.X, rect.Y+rect.H-1, cornerBotLeft, nil, st)
	s.SetContent(rect.X+rect.W-1, rect.Y+rect.H-1, cornerBotRight, nil, st)
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

// overlay is an container that appears over the main UI.
type overlay struct {
	child     Element
	align     string
	prevFocus Element
}

func (o *overlay) MinSize() (int, int) {
	return o.child.MinSize()
}

func (o *overlay) Layout(x, y, w, h int) *LayoutNode {
	mw, mh := o.child.MinSize()
	switch o.align {
	case "top":
		x = x + (w-mw)/2
		// y = y + 1
	case "center":
		fallthrough
	default:
		x = x + (w-mw)/2
		y = y + (h-mh)/2
	}

	node := NewLayoutNode(o, x, y, mw, mh)
	node.Children = []*LayoutNode{o.child.Layout(x, y, mw, mh)}
	return node
}

func (o *overlay) Render(s Screen, rect Rect) {
	ResetRect(s, rect, Style{})
}

// ---------------------------------------------------------------------
// APP RUNNER
// ---------------------------------------------------------------------

type App struct {
	screen  Screen
	Root    Element // root element to render
	focused Element
	// focusID map[string]Element
	hover Element
	tree  *LayoutNode // reflects the view hierarchy after last render
	done  chan struct{}

	clickPoint Point
	bindings   map[string]func()
	overlay    *overlay // for temporary display
}

var app *App

func Default() *App {
	if app == nil {
		app = NewApp(Empty{})
	}
	return app
}

func NewApp(root Element) *App {
	a := &App{
		Root:     root,
		done:     make(chan struct{}),
		bindings: make(map[string]func()),
	}
	a.bindings["Ctrl+C"] = a.Close
	return a
}

func (a *App) Screen() Screen {
	return a.screen
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
	a.tree = a.Root.Layout(0, 0, w, h)
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

// SetFocusID marks the given element with a string identifier for later focus.
// func (a *App) SetFocusID(s string, e Element) {
// 	if a.focusID == nil {
// 		a.focusID = make(map[string]Element)
// 	}
// 	a.focusID[s] = e
// }

// Focus sets focus to the element identified by the given string.
// It is intended to be used with MarkFocus, to decouple elements.
//
//	func (a *App) FocusID(s string) {
//		if s == "" {
//			return
//		}
//		e, ok := a.focusID[s]
//		if !ok {
//			log.Printf("focus id %q not found", s)
//			return
//		}
//		a.Focus(e)
//	}

func (a *App) Focused() Element {
	return a.focused
}

func (a *App) Focus(e Element) {
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
	fe, ok := e.(Focusable)
	if !ok {
		a.focused = nil
	} else {
		fe.OnFocus()
		a.focused = e
	}
	log.Printf("focused: %T", a.focused)

	// dismiss overlay when clicking outside of it
	if node := findNode(a.tree, a.overlay); node != nil {
		if found := findNode(node, e); found == nil {
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

// Serve starts the main event loop.
func (a *App) Serve(root Element) error {
	if root != nil {
		a.Root = root
	}
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
		screen.SetCursorStyle(tcell.CursorStyleSteadyBlock, tcell.GetColor(Theme.Cursor))
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

func (a *App) BindKey(key string, action func()) {
	if key == "" || action == nil {
		return
	}
	a.bindings[key] = action
}

func (a *App) handleKey(ev *tcell.EventKey) {
	log.Printf("key %s", ev.Name())
	if action, ok := a.bindings[ev.Name()]; ok {
		action()
		return
	}
	if a.focused == nil {
		return
	}
	if f, ok := a.focused.(Focusable); ok {
		f.HandleKey(ev)
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
		a.Focus(hit)
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

func (a *App) Close() {
	close(a.done)
}

// Overlay displays an overlay element over the main UI
func (a *App) Overlay(e Element, align string) {
	a.overlay = &overlay{
		child: e,
		align: align,
	}
	if a.focused != nil {
		a.overlay.prevFocus = a.focused
	}
	a.Focus(e)
}

// CloseOverlay removes the overlay element
func (a *App) CloseOverlay() {
	if a.overlay != nil {
		a.Focus(a.overlay.prevFocus)
	}
	a.overlay = nil
}

var Theme ColorTheme

func init() {
	Theme = NewMarianaTheme()
}

type ColorTheme struct {
	Foreground string
	Background string
	Cursor     string
	Border     string
	Hover      string
	Selection  string
	Syntax     SyntaxColor
}

type SyntaxColor struct {
	Keyword      Style
	String       Style
	Comment      Style
	Number       Style
	FunctionName Style
	FunctionCall Style
}

func NewBreakersTheme() ColorTheme {
	return ColorTheme{
		Foreground: "#333333", // grey3
		Background: "#fbffff", // white5 (extremely light cyan-white)
		Cursor:     "#5fb3b3", // blue2 (caret)
		Border:     "#d9e0e4", // white2 (selection_border)
		Hover:      "#dae0e2", // white3
		Selection:  "#dae0e2", // white3 (line_highlight / selection)
		Syntax: SyntaxColor{
			Keyword: Style{
				FG:         "#c594c5", // pink
				FontItalic: true,      // storage.type italic
			},
			String: Style{
				FG: "#89bd82", // green
			},
			Comment: Style{
				FG: "#999999", // grey2
			},
			Number: Style{
				FG: "#fac863", // orange
			},
			FunctionName: Style{
				FG: "#5fb3b3", // blue2 (entity.name.function)
			},
			FunctionCall: Style{
				FG: "#6699cc", // blue (variable.function)
			},
		},
	}
}

func NewMarianaTheme() ColorTheme {
	return ColorTheme{
		Foreground: "#d8dee9", // white3
		Background: "#303841", // blue3
		Cursor:     "#fac863", // orange
		Border:     "#65737e", // blue4 (selection_border)
		Hover:      "#4e5a65",
		Selection:  "#4e5a65", // blue2 (alpha handled by terminal blending)
		Syntax: SyntaxColor{
			Keyword: Style{
				FG:         "#c594c5", // pink
				FontItalic: true,
			},
			String: Style{
				FG: "#99c794", // green
			},
			Comment: Style{
				FG: "#a7adba", // blue6
			},
			Number: Style{
				FG: "#fac863", // orange
			},
			FunctionName: Style{
				FG: "#5fb3b3", // blue5 (entity.name.function)
			},
			FunctionCall: Style{
				FG: "#6699cc", // blue (variable.function)
			},
		},
	}
}

const (
	stateDefault = iota
	stateInString
	stateInRawString
	stateInComment
)

type StyleSpan struct {
	Start int
	End   int // exclusive
	Style Style
}

func highlightGo(line []rune) []StyleSpan {
	var spans []StyleSpan
	state := stateDefault
	start := 0

	for i := 0; i < len(line); {
		r := line[i]
		switch state {
		case stateDefault:
			switch r {
			case '"':
				state = stateInString
				start = i
			case '`':
				state = stateInRawString
				start = i
			case '/':
				if i+1 < len(line) && line[i+1] == '/' {
					state = stateInComment
					start = i
				}
			default:
				if isAlphaNumeric(r) {
					j := i + 1
					// Extract word
					for j < len(line) && isAlphaNumeric(line[j]) {
						j++
					}
					word := string(line[i:j])

					if token.IsKeyword(word) {
						spans = append(spans, StyleSpan{
							Start: i,
							End:   j,
							Style: Theme.Syntax.Keyword,
						})
						i = j // skip over the keyword in the loop
						continue
					}

					// function
					if j < len(line) && line[j] == '(' {
						if i-5 >= 0 && string(line[i-5:i]) == "func " {
							spans = append(spans, StyleSpan{
								Start: i,
								End:   j,
								Style: Theme.Syntax.FunctionName,
							})
						} else {
							spans = append(spans, StyleSpan{
								Start: i,
								End:   j,
								Style: Theme.Syntax.FunctionCall,
							})
						}
						i = j
						continue
					}

					isNumber := true
					for _, c := range word {
						if !unicode.IsDigit(c) {
							isNumber = false
							break
						}
					}
					if isNumber {
						spans = append(spans, StyleSpan{
							Start: i,
							End:   j,
							Style: Theme.Syntax.Number,
						})
						i = j
						continue
					}

					i = j // skip over the keyword in the loop
					continue
				}
			}

		case stateInString:
			if r == '"' {
				spans = append(spans, StyleSpan{
					Start: start,
					End:   i + 1,
					Style: Theme.Syntax.String,
				})
				state = stateDefault
			}
		case stateInRawString:
			if r == '`' {
				spans = append(spans, StyleSpan{
					Start: start,
					End:   i + 1,
					Style: Theme.Syntax.String,
				})
				state = stateDefault
			}
		case stateInComment:
			spans = append(spans, StyleSpan{
				Start: start,
				End:   len(line),
				Style: Theme.Syntax.Comment,
			})
			i = len(line)
			continue
		}
		i++
	}

	return spans
}

// isAlphaNumeric 檢查字元是否為字母、數字或底線
func isAlphaNumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func expandStyles(spans []StyleSpan, base Style, n int) []Style {
	styles := make([]Style, n)
	for i := range styles {
		styles[i] = base
	}
	for _, sp := range spans {
		for i := sp.Start; i < sp.End && i < n; i++ {
			styles[i] = styles[i].Merge(sp.Style)
		}
	}
	return styles
}
