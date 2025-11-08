package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// textEditor is a multi-line editable text area that always shows line numbers.
type textEditor struct {
	content [][]rune
	row     int // Current line index
	col     int // Cursor column index (rune index)
	topRow  int // Index of the first line to display (for scrolling)
	focused bool
	style   Style
	viewH   int // last rendered height
}

// TextEditor creates a multi-line editable text area.
func TextEditor() *textEditor {
	return &textEditor{
		content: [][]rune{{}}, // Start with one empty line of runes
		style:   DefaultStyle,
	}
}

func (t *textEditor) Foreground(c string) *textEditor {
	t.style.FG = tcell.ColorNames[c]
	return t
}
func (t *textEditor) Background(c string) *textEditor {
	t.style.BG = tcell.ColorNames[c]
	return t
}

func (t *textEditor) SetText(s string) {
	lines := strings.Split(s, "\n")
	t.content = make([][]rune, len(lines))
	for i, line := range lines {
		t.content[i] = []rune(line)
	}
	t.row = 0
	t.col = 0
	t.adjustCol()
}

func (t *textEditor) adjustCol() {
	if t.row < len(t.content) {
		lineLen := len(t.content[t.row])
		if t.col > lineLen {
			t.col = lineLen
		}
	}
}

func (t *textEditor) MinSize() (int, int) {
	// Fixed width: 5 columns for line numbers + 20 for content
	return 25, 5
}

func (t *textEditor) Layout(x, y, w, h int) *LayoutNode {
	return &LayoutNode{
		Element: t,
		Rect:    Rect{X: x, Y: y, W: w, H: h},
	}
}

func (t *textEditor) Render(s Screen, rect Rect, style Style) {
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

// implement Focusable interface
func (t *textEditor) Focus()          { t.focused = true }
func (t *textEditor) Unfocus()        { t.focused = false }
func (t *textEditor) IsFocused() bool { return t.focused }

func (t *textEditor) HandleKey(ev *tcell.EventKey) {
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
}

func (t *textEditor) HandleMouse(ev *tcell.EventMouse, rect Rect) {
	if ev.Buttons()&tcell.WheelUp != 0 {
		t.topRow = max(0, t.topRow-3)
		return
	}
	if ev.Buttons()&tcell.WheelDown != 0 {
		t.topRow = min(len(t.content)-1, t.topRow+3)
		return
	}
	if ev.Buttons()&tcell.Button1 == 0 {
		return
	}

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
	x, y := ev.Position()
	targetRow := (y - rect.Y) + t.topRow

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

	clickedX := x - (rect.X + lineNumWidth)
	if clickedX < 0 {
		clickedX = 0
	}

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
}

// adjustScroll ensures the cursor (t.row) is visible on the screen.
func (t *textEditor) adjustScroll() {
	// Scroll down if cursor is below the visible area
	if t.row >= t.topRow+t.viewH {
		t.topRow = t.row - t.viewH + 1
	}
	// Scroll up if cursor is above the visible area
	if t.row < t.topRow {
		t.topRow = t.row
	}
}
