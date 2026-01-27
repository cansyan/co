package ui

import (
	"reflect"
	"testing"
)

func TestNewTextEditor(t *testing.T) {
	e := NewTextEditor()
	if e == nil {
		t.Fatal("NewTextEditor returned nil")
	}
	if len(e.buf) != 1 {
		t.Errorf("expected 1 line, got %d", len(e.buf))
	}
	if len(e.buf[0]) != 0 {
		t.Errorf("expected empty first line, got %d runes", len(e.buf[0]))
	}
}

func TestTextEditor_String(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"single line", "hello", "hello\n"},
		{"multiple lines", "line1\nline2\nline3", "line1\nline2\nline3\n"},
		{"trailing newline", "hello\n", "hello\n"},
		{"empty lines", "\n\n", "\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.input)
			got := e.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTextEditor_SetText(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines int
		wantPos   Pos
	}{
		{"empty", "", 1, Pos{0, 0}},
		{"single line", "hello", 1, Pos{0, 0}},
		{"multiple lines", "line1\nline2", 2, Pos{0, 0}},
		{"trailing newline", "hello\n", 2, Pos{0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.input)
			if len(e.buf) != tt.wantLines {
				t.Errorf("SetText() lines = %d, want %d", len(e.buf), tt.wantLines)
			}
			if e.Pos != tt.wantPos {
				t.Errorf("SetText() pos = %v, want %v", e.Pos, tt.wantPos)
			}
		})
	}
}

func TestTextEditor_SetCursor(t *testing.T) {
	e := NewTextEditor()
	e.SetText("line1\nline2\nline3")

	tests := []struct {
		name     string
		row      int
		col      int
		wantPos  Pos
		wantFail bool
	}{
		{"valid position", 1, 2, Pos{1, 2}, false},
		{"negative row", -1, 0, Pos{0, 0}, true},
		{"negative col", 0, -1, Pos{0, 0}, true},
		{"row out of bounds", 10, 0, Pos{0, 0}, true},
		{"start of file", 0, 0, Pos{0, 0}, false},
		{"end of line", 1, 5, Pos{1, 5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e.SetCursor(tt.row, tt.col)
			if !tt.wantFail && e.Pos != tt.wantPos {
				t.Errorf("SetCursor(%d, %d) pos = %v, want %v", tt.row, tt.col, e.Pos, tt.wantPos)
			}
		})
	}
}

func TestTextEditor_InsertText(t *testing.T) {
	tests := []struct {
		name     string
		initial  string
		pos      Pos
		insert   string
		expected string
		wantPos  Pos
	}{
		{
			name:     "insert at start",
			initial:  "world",
			pos:      Pos{0, 0},
			insert:   "hello ",
			expected: "hello world\n",
			wantPos:  Pos{0, 6},
		},
		{
			name:     "insert in middle",
			initial:  "helloworld",
			pos:      Pos{0, 5},
			insert:   " ",
			expected: "hello world\n",
			wantPos:  Pos{0, 6},
		},
		{
			name:     "insert newline",
			initial:  "hello",
			pos:      Pos{0, 5},
			insert:   "\nworld",
			expected: "hello\nworld\n",
			wantPos:  Pos{1, 5},
		},
		{
			name:     "insert multiple lines",
			initial:  "first",
			pos:      Pos{0, 5},
			insert:   "\nsecond\nthird",
			expected: "first\nsecond\nthird\n",
			wantPos:  Pos{2, 5},
		},
		{
			name:     "empty insert",
			initial:  "hello",
			pos:      Pos{0, 0},
			insert:   "",
			expected: "hello\n",
			wantPos:  Pos{0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.initial)
			e.SetCursor(tt.pos.Row, tt.pos.Col)
			e.InsertText(tt.insert)

			got := e.String()
			if got != tt.expected {
				t.Errorf("InsertText() = %q, want %q", got, tt.expected)
			}
			if e.Pos != tt.wantPos {
				t.Errorf("InsertText() pos = %v, want %v", e.Pos, tt.wantPos)
			}
		})
	}
}

func TestTextEditor_DeleteRange(t *testing.T) {
	tests := []struct {
		name     string
		initial  string
		start    Pos
		end      Pos
		expected string
		wantPos  Pos
	}{
		{
			name:     "delete single line",
			initial:  "hello world",
			start:    Pos{0, 0},
			end:      Pos{0, 6},
			expected: "world\n",
			wantPos:  Pos{0, 0},
		},
		{
			name:     "delete in middle",
			initial:  "hello world",
			start:    Pos{0, 6},
			end:      Pos{0, 11},
			expected: "hello \n",
			wantPos:  Pos{0, 6},
		},
		{
			name:     "delete multiple lines",
			initial:  "line1\nline2\nline3",
			start:    Pos{0, 3},
			end:      Pos{2, 2},
			expected: "linne3\n",
			wantPos:  Pos{0, 3},
		},
		{
			name:     "delete entire line",
			initial:  "line1\nline2\nline3",
			start:    Pos{1, 0},
			end:      Pos{2, 0},
			expected: "line1\nline3\n",
			wantPos:  Pos{1, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.initial)
			e.DeleteRange(tt.start, tt.end)

			got := e.String()
			if got != tt.expected {
				t.Errorf("DeleteRange() = %q, want %q", got, tt.expected)
			}
			if e.Pos != tt.wantPos {
				t.Errorf("DeleteRange() pos = %v, want %v", e.Pos, tt.wantPos)
			}
		})
	}
}

func TestTextEditor_Selection(t *testing.T) {
	e := NewTextEditor()
	e.SetText("line1\nline2\nline3")

	// No selection initially
	_, _, ok := e.Selection()
	if ok {
		t.Error("expected no selection initially")
	}

	// Set selection
	e.SetSelection(Pos{0, 2}, Pos{1, 3})
	start, end, ok := e.Selection()
	if !ok {
		t.Fatal("expected selection")
	}
	if start != (Pos{0, 2}) {
		t.Errorf("start = %v, want {0, 2}", start)
	}
	if end != (Pos{1, 3}) {
		t.Errorf("end = %v, want {1, 3}", end)
	}

	// Clear selection
	e.ClearSelection()
	_, _, ok = e.Selection()
	if ok {
		t.Error("expected no selection after clear")
	}

	// Selection with reversed positions should normalize
	e.SetSelection(Pos{2, 5}, Pos{1, 2})
	start, end, ok = e.Selection()
	if !ok {
		t.Fatal("expected selection")
	}
	if start != (Pos{1, 2}) {
		t.Errorf("normalized start = %v, want {1, 2}", start)
	}
	if end != (Pos{2, 5}) {
		t.Errorf("normalized end = %v, want {2, 5}", end)
	}
}

func TestTextEditor_SelectedText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		start    Pos
		end      Pos
		expected string
	}{
		{
			name:     "single line selection",
			text:     "hello world",
			start:    Pos{0, 0},
			end:      Pos{0, 5},
			expected: "hello",
		},
		{
			name:     "multi line selection",
			text:     "line1\nline2\nline3",
			start:    Pos{0, 2},
			end:      Pos{2, 3},
			expected: "ne1\nline2\nlin",
		},
		{
			name:     "full line",
			text:     "line1\nline2",
			start:    Pos{0, 0},
			end:      Pos{1, 0},
			expected: "line1\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.text)
			e.SetSelection(tt.start, tt.end)
			got := e.SelectedText()
			if got != tt.expected {
				t.Errorf("SelectedText() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTextEditor_WordRangeAtCursor(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		pos       Pos
		wantStart int
		wantEnd   int
		wantOk    bool
	}{
		{
			name:      "cursor in word",
			text:      "hello world",
			pos:       Pos{0, 2},
			wantStart: 0,
			wantEnd:   5,
			wantOk:    true,
		},
		{
			name:      "cursor at start of word",
			text:      "hello world",
			pos:       Pos{0, 0},
			wantStart: 0,
			wantEnd:   5,
			wantOk:    true,
		},
		{
			name:      "cursor at end of word",
			text:      "hello world",
			pos:       Pos{0, 5},
			wantStart: 0,
			wantEnd:   5,
			wantOk:    true,
		},
		{
			name:      "cursor on space",
			text:      "hello world",
			pos:       Pos{0, 5},
			wantStart: 0,
			wantEnd:   5,
			wantOk:    true,
		},
		{
			name:      "underscore word",
			text:      "hello_world",
			pos:       Pos{0, 7},
			wantStart: 0,
			wantEnd:   11,
			wantOk:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.text)
			e.SetCursor(tt.pos.Row, tt.pos.Col)

			start, end, ok := e.WordRangeAtCursor()
			if ok != tt.wantOk {
				t.Errorf("WordRangeAtCursor() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok {
				if start != tt.wantStart {
					t.Errorf("WordRangeAtCursor() start = %d, want %d", start, tt.wantStart)
				}
				if end != tt.wantEnd {
					t.Errorf("WordRangeAtCursor() end = %d, want %d", end, tt.wantEnd)
				}
			}
		})
	}
}

func TestTextEditor_FindNext(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		query    string
		startPos Pos
		wantPos  Pos
		wantSel  bool
	}{
		{
			name:     "find in same line",
			text:     "hello world hello",
			query:    "hello",
			startPos: Pos{0, 0},
			wantPos:  Pos{0, 5},
			wantSel:  true,
		},
		{
			name:     "find next occurrence",
			text:     "hello world hello",
			query:    "hello",
			startPos: Pos{0, 5},
			wantPos:  Pos{0, 17},
			wantSel:  true,
		},
		{
			name:     "find across lines",
			text:     "line1\nfind me\nline3",
			query:    "find",
			startPos: Pos{0, 0},
			wantPos:  Pos{1, 4},
			wantSel:  true,
		},
		{
			name:     "wrap around",
			text:     "hello\nworld",
			query:    "hello",
			startPos: Pos{1, 0},
			wantPos:  Pos{0, 5},
			wantSel:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.text)
			e.SetCursor(tt.startPos.Row, tt.startPos.Col)

			e.FindNext(tt.query)

			if e.Pos != tt.wantPos {
				t.Errorf("FindNext() pos = %v, want %v", e.Pos, tt.wantPos)
			}
			_, _, ok := e.Selection()
			if ok != tt.wantSel {
				t.Errorf("FindNext() has selection = %v, want %v", ok, tt.wantSel)
			}
		})
	}
}

func TestTextEditor_UndoRedo(t *testing.T) {
	e := NewTextEditor()
	e.SetText("initial")

	// Save initial state
	e.SaveEdit()

	// Make a change
	e.SetCursor(0, 7)
	e.InsertText(" text")

	if e.String() != "initial text\n" {
		t.Errorf("after insert = %q, want %q", e.String(), "initial text\n")
	}

	// Undo
	e.Undo()
	if e.String() != "initial\n" {
		t.Errorf("after undo = %q, want %q", e.String(), "initial\n")
	}

	// Redo
	e.Redo()
	if e.String() != "initial text\n" {
		t.Errorf("after redo = %q, want %q", e.String(), "initial text\n")
	}

	// Multiple undo
	e.SaveEdit()
	e.InsertText(" more")
	e.Undo()
	e.Undo()
	if e.String() != "initial\n" {
		t.Errorf("after multiple undo = %q, want %q", e.String(), "initial\n")
	}
}

func TestTextEditor_EnsureVisible(t *testing.T) {
	e := NewTextEditor()
	e.SetText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10")
	e.viewH = 5

	// Row below viewport
	e.offsetY = 0
	e.EnsureVisible(7)
	if e.offsetY < 3 {
		t.Errorf("EnsureVisible(7) offsetY = %d, expected >= 3", e.offsetY)
	}

	// Row above viewport
	e.offsetY = 5
	e.EnsureVisible(2)
	if e.offsetY > 2 {
		t.Errorf("EnsureVisible(2) offsetY = %d, expected <= 2", e.offsetY)
	}
}

func TestTextEditor_CenterRow(t *testing.T) {
	e := NewTextEditor()
	e.SetText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10")
	e.viewH = 5

	e.CenterRow(7)
	expected := 7 - (e.viewH / 2)
	if e.offsetY != expected {
		t.Errorf("CenterRow(7) offsetY = %d, want %d", e.offsetY, expected)
	}
}

func TestVisualColFromLine(t *testing.T) {
	tests := []struct {
		name     string
		line     []rune
		index    int
		expected int
	}{
		{"simple text", []rune("hello"), 3, 3},
		{"with tab at start", []rune("\thello"), 1, 4},
		{"with tab in middle", []rune("hel\tlo"), 4, 4},
		{"multiple tabs", []rune("\t\thello"), 2, 8},
		{"beyond end", []rune("hello"), 10, 5},
		{"empty line", []rune(""), 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visualColFromLine(tt.line, tt.index)
			if got != tt.expected {
				t.Errorf("visualColFromLine(%q, %d) = %d, want %d",
					string(tt.line), tt.index, got, tt.expected)
			}
		})
	}
}

func TestVisualColToLine(t *testing.T) {
	tests := []struct {
		name      string
		line      []rune
		visualCol int
		expected  int
	}{
		{"simple text", []rune("hello"), 3, 3},
		{"with tab at start", []rune("\thello"), 5, 2},
		{"tab boundary", []rune("\thello"), 4, 1},
		{"beyond end", []rune("hello"), 10, 5},
		{"empty line", []rune(""), 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visualColToLine(tt.line, tt.visualCol)
			if got != tt.expected {
				t.Errorf("visualColToLine(%q, %d) = %d, want %d",
					string(tt.line), tt.visualCol, got, tt.expected)
			}
		})
	}
}

func TestPos_Advance(t *testing.T) {
	tests := []struct {
		name     string
		start    Pos
		runes    []rune
		expected Pos
	}{
		{"no newlines", Pos{0, 0}, []rune("hello"), Pos{0, 5}},
		{"one newline", Pos{0, 5}, []rune("\nworld"), Pos{1, 5}},
		{"multiple newlines", Pos{0, 0}, []rune("a\nb\nc"), Pos{2, 1}},
		{"empty", Pos{1, 3}, []rune(""), Pos{1, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.start.Advance(tt.runes)
			if got != tt.expected {
				t.Errorf("Pos.Advance() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSplitRunesByNewline(t *testing.T) {
	tests := []struct {
		name     string
		input    []rune
		expected [][]rune
	}{
		{"no newlines", []rune("hello"), [][]rune{[]rune("hello")}},
		{"one newline", []rune("hello\nworld"), [][]rune{[]rune("hello"), []rune("world")}},
		{"trailing newline", []rune("hello\n"), [][]rune{[]rune("hello"), []rune("")}},
		{"multiple newlines", []rune("a\nb\nc"), [][]rune{[]rune("a"), []rune("b"), []rune("c")}},
		{"empty", []rune(""), [][]rune{[]rune("")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitRunesByNewline(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("splitRunesByNewline(%q) = %v, want %v",
					string(tt.input), got, tt.expected)
			}
		})
	}
}

func TestExpandStyles(t *testing.T) {
	base := Style{FG: "white"}
	spans := []StyleSpan{
		{Start: 2, End: 5, Style: Style{BG: "red"}},
		{Start: 7, End: 9, Style: Style{BG: "blue"}},
	}

	styles := expandStyles(spans, base, 10)

	if len(styles) != 10 {
		t.Errorf("expandStyles() length = %d, want 10", len(styles))
	}

	// Check base style (FG should be preserved from base)
	if styles[0].FG != "white" {
		t.Errorf("styles[0].FG = %q, want %q", styles[0].FG, "white")
	}

	// Check first span (BG should be set, FG from base)
	if styles[3].BG != "red" {
		t.Errorf("styles[3].BG = %q, want %q", styles[3].BG, "red")
	}
	if styles[3].FG != "white" {
		t.Errorf("styles[3].FG = %q, want %q (inherited from base)", styles[3].FG, "white")
	}

	// Check second span
	if styles[7].BG != "blue" {
		t.Errorf("styles[7].BG = %q, want %q", styles[7].BG, "blue")
	}
	if styles[7].FG != "white" {
		t.Errorf("styles[7].FG = %q, want %q (inherited from base)", styles[7].FG, "white")
	}
}

func TestTextEditor_Line(t *testing.T) {
	e := NewTextEditor()
	e.SetText("line1\nline2\nline3")

	line := e.Line(1)
	expected := []rune("line2")
	if !reflect.DeepEqual(line, expected) {
		t.Errorf("Line(1) = %v, want %v", line, expected)
	}

	// Verify it's a clone (modifying doesn't affect buffer)
	line[0] = 'X'
	originalLine := e.buf[1]
	if originalLine[0] == 'X' {
		t.Error("Line() should return a clone, not original buffer")
	}
}

func TestTextEditor_Len(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty", "", 1},
		{"single line", "hello", 1},
		{"multiple lines", "line1\nline2\nline3", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.text)
			got := e.Len()
			if got != tt.expected {
				t.Errorf("Len() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestTextEditor_OnChange(t *testing.T) {
	e := NewTextEditor()
	e.SetText("hello")

	called := false
	e.OnChange(func() {
		called = true
	})

	e.SetCursor(0, 5)
	e.InsertText(" world")

	if !called {
		t.Error("OnChange callback was not called")
	}
}

func TestTextEditor_Dirty(t *testing.T) {
	e := NewTextEditor()
	e.SetText("hello")

	if e.Dirty {
		t.Error("new editor should not be dirty")
	}

	e.SetCursor(0, 5)
	e.InsertText(" world")

	if !e.Dirty {
		t.Error("editor should be dirty after insert")
	}
}

func TestTextEditor_FindClosingBracket(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		openRow  int
		openCol  int
		openChar rune
		wantRow  int
		wantCol  int
	}{
		{
			name:     "simple parens",
			text:     "(hello)",
			openRow:  0,
			openCol:  0,
			openChar: '(',
			wantRow:  0,
			wantCol:  6,
		},
		{
			name:     "nested parens",
			text:     "((hello))",
			openRow:  0,
			openCol:  0,
			openChar: '(',
			wantRow:  0,
			wantCol:  8,
		},
		{
			name:     "multi-line",
			text:     "{\nline1\nline2\n}",
			openRow:  0,
			openCol:  0,
			openChar: '{',
			wantRow:  3,
			wantCol:  0,
		},
		{
			name:     "not found",
			text:     "(hello",
			openRow:  0,
			openCol:  0,
			openChar: '(',
			wantRow:  -1,
			wantCol:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.text)

			gotRow, gotCol := e.findClosingBracket(tt.openRow, tt.openCol, tt.openChar)
			if gotRow != tt.wantRow || gotCol != tt.wantCol {
				t.Errorf("findClosingBracket() = (%d, %d), want (%d, %d)",
					gotRow, gotCol, tt.wantRow, tt.wantCol)
			}
		})
	}
}

func TestTextEditor_FindOpeningBracket(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		startRow int
		startCol int
		wantRow  int
		wantCol  int
		wantChar rune
	}{
		{
			name:     "simple parens",
			text:     "(hello)",
			startRow: 0,
			startCol: 6,
			wantRow:  0,
			wantCol:  0,
			wantChar: '(',
		},
		{
			name:     "nested parens",
			text:     "((hello))",
			startRow: 0,
			startCol: 8,
			wantRow:  0,
			wantCol:  0,
			wantChar: '(',
		},
		{
			name:     "not found",
			text:     "hello)",
			startRow: 0,
			startCol: 5,
			wantRow:  -1,
			wantCol:  -1,
			wantChar: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewTextEditor()
			e.SetText(tt.text)

			gotRow, gotCol, gotChar := e.findOpeningBracket(tt.startRow, tt.startCol)
			if gotRow != tt.wantRow || gotCol != tt.wantCol || gotChar != tt.wantChar {
				t.Errorf("findOpeningBracket() = (%d, %d, %c), want (%d, %d, %c)",
					gotRow, gotCol, gotChar, tt.wantRow, tt.wantCol, tt.wantChar)
			}
		})
	}
}
