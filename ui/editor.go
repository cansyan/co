package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// Highlighter defines a function that returns syntax spans for a line of text.
type Highlighter func(line []rune) []StyleSpan

// editRecord represents a single edit operation that can be undone/redone.
type editRecord struct {
	buf       [][]rune // snapshot of buffer state
	pos       Pos      // cursor position after edit
	anchor    Pos      // selection anchor
	selecting bool     // selection state
}

// TextEditor is a multi-line editable text area.
type TextEditor struct {
	buf     [][]rune // row-major text buffer
	Pos     Pos      // cursor position; also selection end
	goalCol int      // desired visual column when moving vertically

	anchor    Pos // selection anchor (fixed head)
	selecting bool

	offsetY  int // vertical scroll offset (top row)
	viewH    int // last rendered height
	contentX int

	focused bool
	pressed bool // mouse pressed

	style       Style
	highlighter Highlighter

	onChange func()
	Dirty    bool

	// Undo/redo support
	undoStack []editRecord
	redoStack []editRecord
	MergeNext bool // whether to merge next edit with current one

	// Inline suggestions, like code completions but no dropdown,
	// accepted with TAB, quietly shown in gray
	InlineSuggest  bool
	Suggester      func(prefix string) string
	currentSuggest string
}

func NewTextEditor() *TextEditor {
	e := &TextEditor{
		buf: [][]rune{{}}, // Start with one empty line of runes
	}
	return e
}

func (e *TextEditor) SetHighlighter(h Highlighter) {
	e.highlighter = h
}

func (e *TextEditor) Foreground(c string) *TextEditor {
	e.style.FG = c
	return e
}
func (e *TextEditor) Background(c string) *TextEditor {
	e.style.BG = c
	return e
}

func (e *TextEditor) Len() int {
	return len(e.buf)
}

func (e *TextEditor) String() string {
	var sb strings.Builder
	for i, line := range e.buf {
		sb.WriteString(string(line))
		if i == len(e.buf)-1 && len(line) == 0 {
			continue
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (e *TextEditor) SetText(s string) {
	lines := strings.Split(s, "\n")
	e.buf = make([][]rune, len(lines))
	for i, line := range lines {
		e.buf[i] = []rune(line)
	}
	e.Pos = Pos{Row: 0, Col: 0}
	e.adjustCol()
}

// SetCursor moves the cursor and clears any active selection.
func (e *TextEditor) SetCursor(row, col int) {
	if row < 0 || row >= len(e.buf) || col < 0 {
		return
	}
	e.ClearSelection()
	e.Pos = Pos{Row: row, Col: col}
	e.adjustCol()
}

// CenterRow centers the given row vertically in the viewport.
// Used for jump operations (goto line, search results) where you want
// the target line in the middle of the screen for context.
func (e *TextEditor) CenterRow(row int) {
	e.offsetY = row - (e.viewH / 2)
	e.clampScroll()
}

// EnsureVisible ensures the current cursor row is visible with minimal scrolling.
// Used for normal navigation (arrow keys, typing) where you want smooth,
// minimal viewport movement - only scrolls if cursor goes out of bounds.
func (e *TextEditor) EnsureVisible(row int) {
	if e.viewH <= 0 {
		return
	}

	const scrolloff = 1

	// Scroll down if row goes below viewport
	if row >= e.offsetY+e.viewH-scrolloff {
		e.offsetY = row - e.viewH + 1 + scrolloff
	}

	// Scroll up if row goes above viewport
	if row < e.offsetY+scrolloff {
		e.offsetY = row - scrolloff
	}

	e.clampScroll()
}

func (e *TextEditor) clampScroll() {
	maxOffset := max(0, len(e.buf)-e.viewH)
	if e.offsetY > maxOffset {
		e.offsetY = maxOffset
	}
	if e.offsetY < 0 {
		e.offsetY = 0
	}
}

func (e *TextEditor) adjustCol() {
	if e.Pos.Row < len(e.buf) {
		lineLen := len(e.buf[e.Pos.Row])
		if e.Pos.Col > lineLen {
			e.Pos.Col = lineLen
		}
	}
}

func (e *TextEditor) MinSize() (int, int) {
	// Fixed width: 5 columns for line numbers, 20 for content
	return 25, 5
}

func (e *TextEditor) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: e,
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

func (e *TextEditor) drawRune(s tcell.Screen, x, y int, maxWidth int, r rune, visualCol int, style Style) int {
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

func (e *TextEditor) Render(s Screen, rect Rect) {
	e.viewH = rect.H

	// Dynamic width calculation (for proper right-justification)
	numLines := len(e.buf)
	if numLines == 0 {
		numLines = 1
	}
	actualNumDigits := len(strconv.Itoa(numLines))
	lineNumWidth := actualNumDigits + 2
	lineNumStyle := Style{FG: "silver"}

	contentX := rect.X + lineNumWidth + 1
	e.contentX = contentX
	contentW := rect.W - lineNumWidth
	if contentW <= 0 {
		return
	}

	var cursorX, cursorY int
	cursorFound := false
	// Loop over visible rows
	for i := range rect.H {
		row := i + e.offsetY
		if row >= len(e.buf) {
			break
		}

		line := e.buf[row]
		lnStyle := lineNumStyle
		if row == e.Pos.Row {
			cursorFound = true
			cursorX = contentX + visualColFromLine(line, e.Pos.Col)
			cursorY = rect.Y + i
			lnStyle.BG = Theme.Selection
		}

		// draw line number
		lineNum := row + 1
		numStr := fmt.Sprintf("%*d  ", lineNumWidth-1, lineNum)
		DrawString(s, rect.X, rect.Y+i, lineNumWidth, numStr, lnStyle.Apply())

		// draw line content
		var styles []Style
		if e.highlighter != nil {
			spans := e.highlighter(line)
			styles = expandStyles(spans, e.style, len(line))
		}

		visualCol := 0
		y := rect.Y + i
		for col, r := range line {
			charStyle := e.style
			if styles != nil {
				charStyle = styles[col]
			}

			if e.isSelected(Pos{Row: row, Col: col}) {
				charStyle.BG = Theme.Selection
			}
			cells := e.drawRune(s, contentX+visualCol, y, contentW-visualCol, r, visualCol, charStyle)
			visualCol += cells
		}

		// draw inline suggestion
		if e.InlineSuggest && row == e.Pos.Row && e.currentSuggest != "" && e.focused {
			suggestStyle := e.style.Merge(Theme.Syntax.Comment)
			suggestRunes := []rune(e.currentSuggest)
			for _, r := range suggestRunes {
				if visualCol >= contentW {
					break
				}
				cells := e.drawRune(s, contentX+visualCol, y, contentW-visualCol, r, visualCol, suggestStyle)
				visualCol += cells
			}
		}

		// draw line end indicator while selected
		if e.isSelected(Pos{Row: row, Col: len(line)}) {
			charStyle := e.style.Merge(Style{BG: Theme.Selection})
			e.drawRune(s, contentX+visualCol, y, contentW-visualCol, ' ', visualCol, charStyle)
		}
	}

	// Place the cursor
	if e.focused {
		if cursorFound {
			s.ShowCursor(cursorX, cursorY)
		} else {
			// Cursor line is not visible, hide cursor
			s.HideCursor()
		}
	}
}
func (e *TextEditor) OnFocus() { e.focused = true }
func (e *TextEditor) OnBlur()  { e.focused = false }

func (e *TextEditor) HandleKey(ev *tcell.EventKey) (consumed bool) {
	keepVisualCol := false
	defer func() {
		if !keepVisualCol {
			e.goalCol = 0
		}
	}()

	onChange := func() {
		if e.onChange != nil {
			e.onChange()
		}
	}

	consumed = true
	switch ev.Key() {
	case tcell.KeyESC:
		e.ClearSelection()
		e.currentSuggest = ""
	case tcell.KeyUp:
		e.ClearSelection()
		e.currentSuggest = ""
		if ev.Modifiers()&tcell.ModMeta != 0 {
			e.Pos.Row, e.Pos.Col = 0, 0
			e.EnsureVisible(e.Pos.Row)
			return
		}

		keepVisualCol = true
		if e.goalCol == 0 {
			e.goalCol = visualColFromLine(e.buf[e.Pos.Row], e.Pos.Col)
		}
		if e.Pos.Row > 0 {
			e.Pos.Row--
			e.Pos.Col = visualColToLine(e.buf[e.Pos.Row], e.goalCol)
			e.adjustCol()
			e.EnsureVisible(e.Pos.Row)
		}
	case tcell.KeyDown:
		e.ClearSelection()
		e.currentSuggest = ""
		if ev.Modifiers()&tcell.ModMeta != 0 {
			e.Pos.Row, e.Pos.Col = len(e.buf)-1, 0
			e.EnsureVisible(e.Pos.Row)
			return
		}

		keepVisualCol = true
		if e.goalCol == 0 {
			e.goalCol = visualColFromLine(e.buf[e.Pos.Row], e.Pos.Col)
		}
		if e.Pos.Row < len(e.buf)-1 {
			e.Pos.Row++
			e.Pos.Col = visualColToLine(e.buf[e.Pos.Row], e.goalCol)
			e.adjustCol()
			e.EnsureVisible(e.Pos.Row)
		}
	case tcell.KeyLeft:
		e.currentSuggest = ""
		if ev.Modifiers()&tcell.ModMeta != 0 {
			e.ClearSelection()
			firstNonSpace := 0
			for i, ch := range e.buf[e.Pos.Row] {
				if !unicode.IsSpace(ch) {
					firstNonSpace = i
					break
				}
			}
			e.Pos.Col = firstNonSpace
			return
		}
		if start, _, ok := e.Selection(); ok {
			e.Pos = start
			e.ClearSelection()
			return
		}

		if e.Pos.Col > 0 {
			e.Pos.Col--
		} else if e.Pos.Row > 0 {
			e.Pos.Row--
			e.Pos.Col = len(e.buf[e.Pos.Row]) // End of previous line
			e.EnsureVisible(e.Pos.Row)
		}
	case tcell.KeyRight:
		e.currentSuggest = ""
		if ev.Modifiers()&tcell.ModMeta != 0 {
			e.ClearSelection()
			e.Pos.Col = len(e.buf[e.Pos.Row])
			return
		}
		if _, end, ok := e.Selection(); ok {
			e.Pos = end
			e.ClearSelection()
			return
		}
		if e.Pos.Col < len(e.buf[e.Pos.Row]) {
			e.Pos.Col++
		} else if e.Pos.Row < len(e.buf)-1 {
			e.Pos.Row++
			e.Pos.Col = 0 // Start of next line
			e.EnsureVisible(e.Pos.Row)
		}
	case tcell.KeyEnter:
		e.currentSuggest = ""
		e.SaveEdit()
		e.MergeNext = false
		defer onChange()
		e.Dirty = true
		if start, end, ok := e.Selection(); ok {
			e.DeleteRange(start, end)
			e.ClearSelection()
		}
		head := e.buf[e.Pos.Row][:e.Pos.Col]
		tail := e.buf[e.Pos.Row][e.Pos.Col:]

		// keep indentation
		lead := 0
		for _, r := range head {
			if unicode.IsSpace(r) {
				lead++
			} else {
				break
			}
		}
		newLine := make([]rune, lead+len(tail))
		copy(newLine, head[:lead])
		copy(newLine[lead:], tail)

		e.buf[e.Pos.Row] = head
		e.buf = slices.Insert(e.buf, e.Pos.Row+1, newLine)

		e.Pos.Row++
		e.Pos.Col = lead
		e.EnsureVisible(e.Pos.Row)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if !e.MergeNext {
			e.SaveEdit()
		}
		e.MergeNext = true
		defer onChange()
		e.Dirty = true
		if start, end, ok := e.Selection(); ok {
			e.DeleteRange(start, end)
			e.ClearSelection()
			e.updateInlineSuggest()
			return
		}
		if e.Pos.Col > 0 {
			e.buf[e.Pos.Row] = slices.Delete(e.buf[e.Pos.Row], e.Pos.Col-1, e.Pos.Col)
			e.Pos.Col--
			e.updateInlineSuggest()
		} else if e.Pos.Row > 0 {
			prevLine := e.buf[e.Pos.Row-1]
			e.Pos.Col = len(prevLine)
			e.buf[e.Pos.Row-1] = append(prevLine, e.buf[e.Pos.Row]...)

			e.buf = slices.Delete(e.buf, e.Pos.Row, e.Pos.Row+1)
			e.Pos.Row--
			e.EnsureVisible(e.Pos.Row)
			e.currentSuggest = ""
		}
	case tcell.KeyRune:
		if !e.MergeNext {
			e.SaveEdit()
		}
		e.MergeNext = true
		defer onChange()
		e.Dirty = true
		// 如果有選取，先刪除選取範圍，再插入字元
		if start, end, ok := e.Selection(); ok {
			e.DeleteRange(start, end)
			e.ClearSelection()
		}
		r := ev.Rune()
		e.buf[e.Pos.Row] = slices.Insert(e.buf[e.Pos.Row], e.Pos.Col, r)
		e.Pos.Col++
		e.updateInlineSuggest()
	case tcell.KeyTAB:
		// Try to accept inline suggestion first
		if e.InlineSuggest && e.currentSuggest != "" {
			e.SaveEdit()
			e.MergeNext = false
			defer onChange()
			e.Dirty = true
			e.InsertText(e.currentSuggest)
			e.currentSuggest = ""
			return true
		}

		e.SaveEdit()
		e.MergeNext = false
		defer onChange()
		e.Dirty = true
		if start, end, ok := e.Selection(); ok {
			e.DeleteRange(start, end)
			e.ClearSelection()
		}
		e.buf[e.Pos.Row] = slices.Insert(e.buf[e.Pos.Row], e.Pos.Col, '\t')
		e.Pos.Col++
	case tcell.KeyHome:
		// goto the first non-space character
		for i, char := range e.buf[e.Pos.Row] {
			if !unicode.IsSpace(char) {
				e.Pos.Col = i
				break
			}
		}
	case tcell.KeyEnd:
		e.Pos.Col = len(e.buf[e.Pos.Row])
	default:
		consumed = false
	}
	return consumed
}

func (e *TextEditor) OnMouseUp(x, y int) {
	e.pressed = false
	if e.Pos.Row == e.anchor.Row && e.Pos.Col == e.anchor.Col {
		e.selecting = false
	}
}

func (e *TextEditor) OnMouseDown(x, y int) {
	e.currentSuggest = ""
	// Calculate the target row (relative to content)
	targetRow := y + e.offsetY

	// Clamp the target row
	if targetRow < 0 {
		e.Pos.Row = 0
	} else if targetRow >= len(e.buf) {
		e.Pos.Row = len(e.buf) - 1
	} else {
		e.Pos.Row = targetRow
	}

	if e.Pos.Row < 0 {
		return
	}

	// Calculate the target column (rune index)
	visualCol := max(x-e.contentX, 0)
	e.Pos.Col = visualColToLine(e.buf[e.Pos.Row], visualCol)

	if !e.pressed {
		// 點擊瞬間，錨點與游標重合
		e.anchor = e.Pos
		e.selecting = true
		e.pressed = true
	}
}

func (e *TextEditor) OnMouseEnter() {}
func (e *TextEditor) OnMouseLeave() {}
func (e *TextEditor) OnMouseMove(lx, ly int) {
	if e.pressed {
		// Drag to select
		targetRow := ly + e.offsetY
		if targetRow < 0 {
			targetRow = 0
		} else if targetRow >= len(e.buf) {
			targetRow = len(e.buf) - 1
		}
		currentLine := e.buf[targetRow]
		clickedX := max(lx-e.contentX, 0)
		targetCol := visualColToLine(currentLine, clickedX)

		e.Pos.Row = targetRow
		e.Pos.Col = targetCol
	}
}

// SetSelection sets the selection anchor to startRow and startCol,
// sets content cursor to endRow and endCol.
func (e *TextEditor) SetSelection(start, end Pos) {
	length := e.Len()
	if start.Row < 0 || start.Row > length-1 || end.Row < 0 || end.Row > length-1 {
		return
	}
	e.anchor = start
	e.Pos = end
	e.selecting = true
	e.adjustCol()
}

func (e *TextEditor) Selection() (start, end Pos, ok bool) {
	if !e.selecting || (e.Pos.Row == e.anchor.Row && e.Pos.Col == e.anchor.Col) {
		return
	}

	start = e.anchor
	end = e.Pos

	// ensure start is before end
	if start.Row > end.Row || (start.Row == end.Row && start.Col > end.Col) {
		start, end = end, start
	}
	return start, end, true
}

func (e *TextEditor) ClearSelection() {
	e.selecting = false
}

func (e *TextEditor) isSelected(pos Pos) bool {
	start, end, ok := e.Selection()
	if !ok {
		return false
	}

	if pos.Row < start.Row || pos.Row > end.Row {
		return false
	}
	if pos.Row > start.Row && pos.Row < end.Row {
		return true
	}
	if start.Row == end.Row {
		return pos.Col >= start.Col && pos.Col < end.Col
	}
	if pos.Row == start.Row {
		return pos.Col >= start.Col
	}
	if pos.Row == end.Row {
		return pos.Col < end.Col
	}
	return false
}

// WordRangeAtCursor returns the word boundaries at current cursor.
func (e *TextEditor) WordRangeAtCursor() (start, end int, ok bool) {
	if e.Pos.Row < 0 || e.Pos.Row >= len(e.buf) {
		return
	}
	line := e.buf[e.Pos.Row]
	if e.Pos.Col < 0 || e.Pos.Col > len(line) {
		return
	}

	isWordChar := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}

	start = e.Pos.Col
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}

	end = e.Pos.Col
	for end < len(line) && isWordChar(line[end]) {
		end++
	}

	// no word
	if start == end {
		return
	}

	return start, end, true
}

// SelectWord selects word at current cursor
func (e *TextEditor) SelectWord() {
	if len(e.buf) == 0 || e.Pos.Row >= len(e.buf) {
		return
	}

	start, end, ok := e.WordRangeAtCursor()
	if !ok {
		return
	}
	// 讓游標停在單詞末尾，方便下次搜尋從末尾開始
	e.SetSelection(Pos{Row: e.Pos.Row, Col: start}, Pos{Row: e.Pos.Row, Col: end})
}

// ExpandSelectionToLine expands selection to line.
// Repeated calls may expand further lines.
func (e *TextEditor) ExpandSelectionToLine() {
	if len(e.buf) == 0 || e.Pos.Row >= len(e.buf) {
		return
	}

	start, end, ok := e.Selection()
	if !ok {
		if e.Pos.Row < len(e.buf)-1 {
			// 選中整行，並將游標移至下一行開頭（模仿主流編輯器行為）
			e.SetSelection(Pos{Row: e.Pos.Row}, Pos{Row: e.Pos.Row + 1})
		} else {
			e.SetSelection(Pos{Row: e.Pos.Row}, Pos{Row: e.Pos.Row, Col: len(e.buf[e.Pos.Row])})
		}
		return
	}

	// expand selection
	if end.Row < e.Len()-1 {
		e.SetSelection(Pos{Row: start.Row, Col: 0}, Pos{Row: end.Row + 1, Col: 0})
	} else {
		line := e.Line(end.Row)
		e.SetSelection(Pos{Row: start.Row, Col: 0}, Pos{Row: end.Row, Col: len(line)})
	}
}

// ExpandSelectionToBrackets expands selection to the nearest enclosing brackets.
// Repeated calls may expand further depending on context;
func (e *TextEditor) ExpandSelectionToBrackets() {
	openRow, openCol, openCh := e.findOpeningBracket(e.Pos.Row, e.Pos.Col)
	if openCol == -1 {
		return
	}

	closeRow, closeCol := e.findClosingBracket(openRow, openCol, openCh)
	if closeCol == -1 {
		return
	}

	// include the brackets
	e.SetSelection(Pos{Row: openRow, Col: openCol}, Pos{Row: closeRow, Col: closeCol + 1})
}

var bracketOpen = map[rune]rune{
	'(': ')',
	'[': ']',
	'{': '}',
}

var bracketClose = map[rune]rune{
	')': '(',
	']': '[',
	'}': '{',
}

func (e *TextEditor) findOpeningBracket(startRow, startCol int) (openRow, openCol int, openCh rune) {
	var stack []rune
	for r := startRow; r >= 0; r-- {
		cStart := len(e.buf[r]) - 1
		if r == startRow {
			cStart = startCol - 1
		}

		for c := cStart; c >= 0; c-- {
			char := e.buf[r][c]
			if open, ok := bracketClose[char]; ok {
				stack = append(stack, open)
			} else if _, ok := bracketOpen[char]; ok {
				if len(stack) == 0 {
					return r, c, char
				}
				// pop
				stack = stack[:len(stack)-1]
			}
		}
	}
	return -1, -1, 0
}

func (e *TextEditor) findClosingBracket(openRow, openCol int, openCh rune) (closeRow, closeCol int) {
	closeCh := bracketOpen[openCh]
	depth := 0
	for r := openRow; r < len(e.buf); r++ {
		cStart := 0
		if r == openRow {
			cStart = openCol + 1
		}

		for c := cStart; c < len(e.buf[r]); c++ {
			char := e.buf[r][c]
			switch char {
			case openCh:
				depth++
			case closeCh:
				if depth == 0 {
					return r, c
				}
				depth--
			}
		}
	}
	return -1, -1
}

// FindNext 尋找下一個匹配項並更新選區
func (e *TextEditor) FindNext(query string) {
	if query == "" {
		return
	}
	qRunes := []rune(query)
	qLen := len(qRunes)
	lineCount := len(e.buf)

	// 從當前位置之後開始搜尋
	startRow := e.Pos.Row
	startCol := e.Pos.Col

	for i := range lineCount {
		// 使用取模實現 Wrap Around (循環搜尋)
		currentRow := (startRow + i) % lineCount
		line := e.buf[currentRow]

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
			for k := range qLen {
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
			e.anchor = Pos{Row: currentRow, Col: actualCol}
			e.Pos.Row = currentRow
			e.Pos.Col = actualCol + qLen
			e.selecting = true

			// 3. 確保視覺調整
			e.CenterRow(currentRow)
			return
		}
	}
}

// SelectedText 回傳當前選區的字串內容
func (e *TextEditor) SelectedText() string {
	start, end, ok := e.Selection()
	if !ok {
		return ""
	}

	if start.Row == end.Row {
		return string(e.buf[start.Row][start.Col:end.Col])
	}

	var sb strings.Builder
	sb.WriteString(string(e.buf[start.Row][start.Col:]))
	sb.WriteByte('\n')
	for i := start.Row + 1; i < end.Row; i++ {
		sb.WriteString(string(e.buf[i]))
		sb.WriteByte('\n')
	}
	sb.WriteString(string(e.buf[end.Row][:end.Col]))
	return sb.String()
}

func (e *TextEditor) OnScroll(dy int) {
	if len(e.buf) <= e.viewH {
		e.offsetY = 0
	} else if dy < 0 {
		// scroll down
		e.offsetY = max(e.offsetY+dy, 0)
	} else {
		// scroll up
		e.offsetY = min(e.offsetY+dy, len(e.buf)-e.viewH)
	}
}

// OnChange sets a callback function that is called whenever the text content changes.
func (e *TextEditor) OnChange(fn func()) {
	e.onChange = fn
}

// Line return a line of text on the given row index.
func (e *TextEditor) Line(i int) []rune {
	return slices.Clone(e.buf[i])
}

type Pos struct {
	Row int
	Col int
}

func (p Pos) Advance(rs []rune) Pos {
	for _, r := range rs {
		if r == '\n' {
			p.Row++
			p.Col = 0
		} else {
			p.Col++
		}
	}
	return p
}

func (e *TextEditor) insertRunes(pos Pos, rs []rune) {
	if len(rs) == 0 {
		return
	}

	lines := splitRunesByNewline(rs)
	line := e.buf[pos.Row]

	if len(lines) == 1 {
		e.buf[pos.Row] = spliceRunes(line, pos.Col, 0, lines[0])
		return
	}

	// multi-line insert
	head := append(append([]rune{}, line[:pos.Col]...), lines[0]...)
	tail := append([]rune{}, line[pos.Col:]...)

	lines[len(lines)-1] = append(lines[len(lines)-1], tail...)

	newLines := make([][]rune, 0, len(lines))
	newLines = append(newLines, head)
	newLines = append(newLines, lines[1:]...)

	e.buf = spliceLines(e.buf, pos.Row, 1, newLines)
}

// InsertText simulates a paste operation: it inserts a string 's' at the current
// cursor position (t.row, t.col), correctly handling any embedded newlines ('\n').
func (e *TextEditor) InsertText(s string) {
	if s == "" {
		return
	}

	// selection
	if start, end, ok := e.Selection(); ok {
		e.DeleteRange(start, end)
		e.ClearSelection()
	}

	// insert
	rs := []rune(s)
	e.insertRunes(e.Pos, rs)

	// move cursor
	e.Pos = e.Pos.Advance(rs)

	// UI side effects
	e.EnsureVisible(e.Pos.Row)
	e.Dirty = true
	if e.onChange != nil {
		e.onChange()
	}
}

func splitRunesByNewline(rs []rune) [][]rune {
	var lines [][]rune
	start := 0
	for i, r := range rs {
		if r == '\n' {
			lines = append(lines, append([]rune{}, rs[start:i]...))
			start = i + 1
		}
	}
	lines = append(lines, append([]rune{}, rs[start:]...))
	return lines
}

func spliceRunes(rs []rune, start, remove int, insert []rune) []rune {
	out := make([]rune, 0, len(rs)-remove+len(insert))
	out = append(out, rs[:start]...)
	if insert != nil {
		out = append(out, insert...)
	}
	out = append(out, rs[start+remove:]...)
	return out
}

func spliceLines(lines [][]rune, start, remove int, insert [][]rune) [][]rune {
	out := make([][]rune, 0, len(lines)-remove+len(insert))
	out = append(out, lines[:start]...)
	if insert != nil {
		out = append(out, insert...)
	}
	out = append(out, lines[start+remove:]...)
	return out
}

// DeleteRange deletes a range of text defined by two cursor positions (start, end).
// the positions are inclusive of start and exclusive of end.
func (e *TextEditor) DeleteRange(start, end Pos) {
	// 1. Normalize and clamp the selection range
	if start.Row > end.Row || (start.Row == end.Row && start.Col > end.Col) {
		start.Row, end.Row = end.Row, start.Row
		start.Col, end.Col = end.Col, start.Col
	}

	if start.Row < 0 {
		start.Row = 0
	}
	if end.Row >= len(e.buf) {
		end.Row = len(e.buf) - 1
	}
	if start.Row > end.Row {
		return
	}

	startLine := e.buf[start.Row]
	endLine := e.buf[end.Row]

	start.Col = min(start.Col, len(startLine))
	end.Col = min(end.Col, len(endLine))

	// 2. Extract Head and Tail
	head := startLine[:start.Col]
	tail := endLine[end.Col:]

	// 3. Perform Merging and Deletion
	mergedLine := append(head, tail...)

	// a) Replace the starting line with the merged content
	e.buf[start.Row] = mergedLine

	// b) Delete intermediate lines
	if start.Row < end.Row {
		e.buf = slices.Delete(e.buf, start.Row+1, end.Row+1)
	}

	if len(e.buf) == 0 {
		e.buf = [][]rune{{}}
	}

	// 4. Update Cursor State
	e.Pos.Row = start.Row
	e.Pos.Col = len(head)

	e.EnsureVisible(e.Pos.Row)
	e.Dirty = true
	if e.onChange != nil {
		e.onChange()
	}
}

// SaveEdit saves the current buffer state to the undo stack.
func (e *TextEditor) SaveEdit() {
	// Create a deep copy of the buffer
	bufCopy := make([][]rune, len(e.buf))
	for i := range e.buf {
		bufCopy[i] = slices.Clone(e.buf[i])
	}

	record := editRecord{
		buf:       bufCopy,
		pos:       e.Pos,
		anchor:    e.anchor,
		selecting: e.selecting,
	}

	e.undoStack = append(e.undoStack, record)

	// Clear redo stack when new edit is made
	e.redoStack = nil
}

// Undo reverts the last edit operation.
func (e *TextEditor) Undo() {
	if len(e.undoStack) == 0 {
		return
	}

	// Save current state to redo stack
	bufCopy := make([][]rune, len(e.buf))
	for i := range e.buf {
		bufCopy[i] = slices.Clone(e.buf[i])
	}
	e.redoStack = append(e.redoStack, editRecord{
		buf:       bufCopy,
		pos:       e.Pos,
		anchor:    e.anchor,
		selecting: e.selecting,
	})

	// Restore previous state
	record := e.undoStack[len(e.undoStack)-1]
	e.undoStack = e.undoStack[:len(e.undoStack)-1]

	e.buf = record.buf
	e.Pos = record.pos
	e.anchor = record.anchor
	e.selecting = record.selecting

	e.adjustCol()
	e.EnsureVisible(e.Pos.Row)
	e.MergeNext = false
	if e.onChange != nil {
		e.onChange()
	}
}

// Redo reapplies an undone edit operation.
func (e *TextEditor) Redo() {
	if len(e.redoStack) == 0 {
		return
	}

	// Save current state to undo stack
	bufCopy := make([][]rune, len(e.buf))
	for i := range e.buf {
		bufCopy[i] = slices.Clone(e.buf[i])
	}
	e.undoStack = append(e.undoStack, editRecord{
		buf:       bufCopy,
		pos:       e.Pos,
		anchor:    e.anchor,
		selecting: e.selecting,
	})

	// Restore redo state
	record := e.redoStack[len(e.redoStack)-1]
	e.redoStack = e.redoStack[:len(e.redoStack)-1]

	e.buf = record.buf
	e.Pos = record.pos
	e.anchor = record.anchor
	e.selecting = record.selecting

	e.adjustCol()
	e.EnsureVisible(e.Pos.Row)
	e.MergeNext = false
	if e.onChange != nil {
		e.onChange()
	}
}

// updateInlineSuggest finds and sets the current inline suggestion.
func (e *TextEditor) updateInlineSuggest() {
	if !e.InlineSuggest || e.Suggester == nil {
		return
	}

	e.currentSuggest = ""

	if e.Pos.Row >= len(e.buf) || e.Pos.Col == 0 {
		return
	}

	line := e.buf[e.Pos.Row]
	if e.Pos.Col > len(line) {
		return
	}

	// find word start
	isWordChar := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}

	wordStart := e.Pos.Col
	for wordStart > 0 && isWordChar(line[wordStart-1]) {
		wordStart--
	}

	if wordStart == e.Pos.Col {
		return
	}

	prefix := string(line[wordStart:e.Pos.Col])
	if len(prefix) == 0 {
		return
	}

	e.currentSuggest = e.Suggester(prefix)
}

type StyleSpan struct {
	Start int
	End   int // exclusive
	Style Style
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
