package main

import (
	"context"
	"go/token"
	"strings"
	"unicode"

	"github.com/cansyan/co/ui"
	"github.com/gdamore/tcell/v2"
)

type Editor struct {
	*ui.Editor
	app     *App
	symbols []symbol
}

func NewEditor(r *App) *Editor {
	e := &Editor{
		Editor: ui.NewTextEditor(),
		app:    r,
	}

	e.InlineSuggest = true
	e.Suggester = func(ctx context.Context, prefix string) string {
		if len(prefix) < 2 {
			// avoid abusing suggestions for short prefixes
			return ""
		}

		prefix = strings.ToLower(prefix)
		for _, s := range e.symbols {
			if len(s.Name) > len(prefix) && strings.HasPrefix(strings.ToLower(s.Name), prefix) {
				return s.Name
			}
		}
		return ""
	}
	return e
}

func (e *Editor) Layout(r ui.Rect) *ui.Node {
	return &ui.Node{
		Element: e,
		Rect:    r,
	}
}

// HandleKey handles editor-specific keybindings.
// If the key is not handled here, it will bubble up to the app level.
func (e *Editor) HandleKey(ev *tcell.EventKey) bool {
	switch strings.ToLower(ev.Name()) {
	case "ctrl+z":
		e.Editor.Undo()
	case "ctrl+y":
		e.Editor.Redo()
	case "ctrl+a":
		lastLine := e.Line(e.Len() - 1)
		e.SetSelection(ui.Pos{}, ui.Pos{Row: e.Len() - 1, Col: len(lastLine)})
	case "ctrl+c":
		s := e.SelectedText()
		if s == "" {
			// copy current line by default
			s = string(e.Line(e.Pos.Row))
		}
		e.app.clipboard = s
		e.app.manager.Screen().SetClipboard([]byte(s))
	case "ctrl+x":
		e.Editor.SaveEdit()
		e.MergeNext = false
		start, end, ok := e.Selection()
		if !ok {
			// cut line by default
			s := string(e.Line(e.Pos.Row)) + "\n"
			e.app.clipboard = s
			e.app.manager.Screen().SetClipboard([]byte(s))
			e.DeleteRange(ui.Pos{Row: e.Pos.Row}, ui.Pos{Row: e.Pos.Row + 1})
			return true
		}

		s := e.SelectedText()
		e.app.clipboard = s
		e.app.manager.Screen().SetClipboard([]byte(s))
		e.DeleteRange(start, end)
		e.ClearSelection()
	case "ctrl+v":
		e.Editor.SaveEdit()
		e.MergeNext = false
		e.InsertText(e.app.clipboard)
	case "ctrl+d":
		// if no selection, select the word at current cursor;
		// if has selection, jump to the next same word, like * in Vim.
		// To keep things simple, this is not multiple selection (multi-cursor)
		start, end, ok := e.Selection()
		if !ok {
			e.SelectWord()
		} else if start.Row == end.Row {
			query := string(e.Line(start.Row)[start.Col:end.Col])
			e.FindNext(query)
		}
	case "ctrl+l":
		e.ExpandSelectionToLine()
	case "ctrl+b":
		e.ExpandSelectionToBrackets()
	case "ctrl+g":
		e.gotoDefinition()
	case "alt+up": // goto first line
		e.gotoLine(0)
	case "alt+down": // goto last line
		e.gotoLine(e.Len() - 1)
	case "alt+left": // goto the first non-space character of line
		e.ClearSelection()
		for i, char := range e.Line(e.Pos.Row) {
			if !unicode.IsSpace(char) {
				e.Pos.Col = i
				break
			}
		}
	case "alt+right": // goto the end of line
		e.ClearSelection()
		e.Pos.Col = len(e.Line(e.Pos.Row))
	default:
		if !e.Editor.HandleKey(ev) {
			// Bubble event to parent
			return e.app.handleGlobalKey(ev)
		}
	}
	return true
}

// gotoLine moves the cursor to the specified 0-based line number
// and centers the view on that line.
func (e *Editor) gotoLine(line int) {
	if line < 0 {
		line = 0
	}
	if line >= e.Len() {
		line = e.Len() - 1
	}
	e.SetCursor(line, 0)
	e.CenterRow(line)
	e.app.recordJump()
}

func (e *Editor) gotoDefinition() {
	start, end, ok := e.WordRangeAtCursor()
	if !ok {
		return
	}
	word := string(e.Line(e.Pos.Row)[start:end])

	for _, s := range e.symbols {
		if s.Name == word {
			e.gotoLine(s.Line)
			return
		}
	}
}

func (e *Editor) updateSymbols() {
	e.symbols = extractSymbols(e.String())
}

func (e *Editor) OnMouseDown(lx, ly int) {
	e.Editor.OnMouseDown(lx, ly)
	e.app.recordJump()
}

const (
	stateDefault = iota
	stateInString
	stateInRawString
	stateInComment
)

func highlightGo(line []rune) []ui.StyleSpan {
	var spans []ui.StyleSpan
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
				} else {
					spans = append(spans, ui.StyleSpan{
						Start: i,
						End:   i + 1,
						Style: ui.Theme.Syntax.Operator,
					})
					i++
					continue
				}
			case '+', '-', '*', '%', '&', '|', '^', '<', '>', '=', '!', ':':
				// Parse multi-character operators
				j := i + 1
				for j < len(line) && isOperatorRune(line[j]) {
					j++
				}
				spans = append(spans, ui.StyleSpan{
					Start: i,
					End:   j,
					Style: ui.Theme.Syntax.Operator,
				})
				i = j
				continue
			default:
				if isAlphaNumeric(r) {
					j := i + 1
					for j < len(line) && isAlphaNumeric(line[j]) {
						j++
					}
					word := string(line[i:j])

					if token.IsKeyword(word) {
						spans = append(spans, ui.StyleSpan{
							Start: i,
							End:   j,
							Style: ui.Theme.Syntax.Keyword,
						})
						i = j
						continue
					}

					if j < len(line) && line[j] == '(' {
						style := ui.Theme.Syntax.FunctionCall
						if i-5 >= 0 && string(line[i-5:i]) == "func " {
							style = ui.Theme.Syntax.FunctionName
						}
						spans = append(spans, ui.StyleSpan{
							Start: i,
							End:   j,
							Style: style,
						})
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
						spans = append(spans, ui.StyleSpan{
							Start: i,
							End:   j,
							Style: ui.Theme.Syntax.Number,
						})
						i = j
						continue
					}
					i = j
					continue
				}
			}
		case stateInString:
			if r == '"' {
				spans = append(spans, ui.StyleSpan{Start: start, End: i + 1, Style: ui.Theme.Syntax.String})
				state = stateDefault
			}
		case stateInRawString:
			if r == '`' {
				spans = append(spans, ui.StyleSpan{Start: start, End: i + 1, Style: ui.Theme.Syntax.String})
				state = stateDefault
			}
		case stateInComment:
			spans = append(spans, ui.StyleSpan{Start: start, End: len(line), Style: ui.Theme.Syntax.Comment})
			return spans
		}
		i++
	}
	return spans
}

func isAlphaNumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func isOperatorRune(r rune) bool {
	return strings.ContainsRune("+-*/%&|^<>=!:", r)
}

func highlightMarkdown(line []rune) []ui.StyleSpan {
	var spans []ui.StyleSpan
	if len(line) == 0 {
		return spans
	}

	// Headers: # ## ### etc.
	if line[0] == '#' {
		i := 0
		for i < len(line) && line[i] == '#' {
			i++
		}
		spans = []ui.StyleSpan{
			{
				Start: 0,
				End:   i,
				Style: ui.Style{FontBold: true, FG: ui.Theme.Syntax.Operator.FG},
			},
			{
				Start: i,
				End:   len(line),
				Style: ui.Style{FontBold: true},
			},
		}
		return spans
	}

	if strings.HasPrefix(string(line), "```") {
		spans = []ui.StyleSpan{
			{
				Start: 0,
				End:   3,
				Style: ui.Style{BG: ui.Theme.Selection},
			},
		}
		return spans
	}

	// List items: -, *, or digits followed by .
	trimmed := 0
	for trimmed < len(line) && unicode.IsSpace(line[trimmed]) {
		trimmed++
	}
	if trimmed < len(line) {
		if line[trimmed] == '-' || line[trimmed] == '*' {
			if trimmed+1 >= len(line) || unicode.IsSpace(line[trimmed+1]) {
				spans = append(spans, ui.StyleSpan{
					Start: trimmed,
					End:   trimmed + 1,
					Style: ui.Theme.Syntax.Number,
				})
			}
		} else if unicode.IsDigit(line[trimmed]) {
			j := trimmed + 1
			for j < len(line) && unicode.IsDigit(line[j]) {
				j++
			}
			if j < len(line) && line[j] == '.' {
				spans = append(spans, ui.StyleSpan{
					Start: trimmed,
					End:   j + 1,
					Style: ui.Theme.Syntax.Keyword,
				})
			}
		}
	}

	// Inline code: `code`
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			start := i
			i++
			for i < len(line) && line[i] != '`' {
				i++
			}
			if i < len(line) {
				spans = append(spans, ui.StyleSpan{
					Start: start,
					End:   i + 1,
					Style: ui.Style{BG: ui.Theme.Selection},
				})
			}
		}
	}

	// Bold: **text**
	for i := 0; i < len(line)-1; i++ {
		if line[i] == '*' && line[i+1] == '*' {
			start := i
			i += 2
			for i < len(line)-1 {
				if line[i] == '*' && line[i+1] == '*' {
					spans = append(spans, ui.StyleSpan{
						Start: start,
						End:   i + 2,
						Style: ui.Style{FontBold: true},
					})
					i++
					break
				}
				i++
			}
		}
	}

	// Italic: *text* (but not **)
	for i := 0; i < len(line); i++ {
		if line[i] == '*' {
			if i > 0 && line[i-1] == '*' {
				continue
			}
			if i+1 < len(line) && line[i+1] == '*' {
				continue
			}
			start := i
			i++
			for i < len(line) {
				if line[i] == '*' {
					if i+1 < len(line) && line[i+1] == '*' {
						break
					}
					spans = append(spans, ui.StyleSpan{
						Start: start,
						End:   i + 1,
						Style: ui.Style{FontItalic: true},
					})
					break
				}
				i++
			}
		}
	}

	// Links: [text](url)
	for i := 0; i < len(line); i++ {
		if line[i] == '[' {
			start := i
			i++
			for i < len(line) && line[i] != ']' {
				i++
			}
			if i < len(line) && i+1 < len(line) && line[i+1] == '(' {
				i += 2
				for i < len(line) && line[i] != ')' {
					i++
				}
				if i < len(line) {
					spans = append(spans, ui.StyleSpan{
						Start: start,
						End:   i + 1,
						Style: ui.Theme.Syntax.FunctionCall,
					})
				}
			}
		}
	}

	return spans
}
