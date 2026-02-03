package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

type Text struct {
	Style   Style
	Content string
}

func NewText(text string) *Text {
	return &Text{Content: text}
}

func (t *Text) Size() (int, int) {
	lines := t.lines()
	maxW := 0
	for _, line := range lines {
		if w := runewidth.StringWidth(line); w > maxW {
			maxW = w
		}
	}
	return maxW, len(lines)
}

func (t *Text) lines() []string {
	return strings.Split(t.Content, "\n")
}

func (t *Text) Layout(r Rect) *Node {
	return &Node{
		Element: t,
		Rect:    r,
	}
}
func (t *Text) Draw(s Screen, rect Rect) {
	lines := t.lines()
	for i, line := range lines {
		if i >= rect.H {
			break
		}
		DrawString(s, rect.X, rect.Y+i, rect.W, line, t.Style)
	}
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

func (b *Button) Size() (int, int) { return runewidth.StringWidth(b.Text) + 2, 1 }
func (b *Button) Layout(r Rect) *Node {
	return &Node{
		Element: b,
		Rect:    r,
	}
}
func (b *Button) Draw(s Screen, rect Rect) {
	st := b.Style
	if !b.NoFeedback && b.hovered {
		st.BG = Theme.Hover
	}
	if !b.NoFeedback && b.pressed {
		st.BG = Theme.Selection
	}
	label := " " + b.Text + " "
	DrawString(s, rect.X, rect.Y, rect.W, label, st)
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

// Input is a single-line text input field.
// The zero value for Input is ready to use.
type Input struct {
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

// String returns the current text content
func (t *Input) String() string {
	return string(t.text)
}

func (t *Input) SetText(s string) {
	t.text = []rune(s)
	t.cursor = len(t.text)
	t.anchor = t.cursor
	if t.OnChange != nil {
		t.OnChange()
	}
}

func (t *Input) Size() (int, int) {
	return 10, 1
}

func (t *Input) Layout(r Rect) *Node {
	return &Node{
		Element: t,
		Rect:    r,
	}
}

func (t *Input) Draw(s Screen, rect Rect) {
	if t.focused && t.cursor < rect.W {
		s.ShowCursor(rect.X+t.cursor, rect.Y)
	}
	// placeholder
	if len(t.text) == 0 {
		DrawString(s, rect.X, rect.Y, rect.W, t.Placeholder, Theme.Syntax.Comment)
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

	// fill remaining space
	for x := xOffset; x < rect.W; x++ {
		s.SetContent(rect.X+x, rect.Y, ' ', nil, baseStyle)
	}
}

func (t *Input) OnFocus() { t.focused = true }
func (t *Input) OnBlur()  { t.focused = false }
func (t *Input) HandleKey(ev *tcell.EventKey) bool {
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

func (t *Input) OnMouseDown(x, y int) {
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

func (t *Input) OnMouseMove(x, y int) {
	if t.pressed {
		t.cursor = t.clampCursor(x)
		if t.OnChange != nil {
			t.OnChange()
		}
	}
}

func (t *Input) OnMouseUp(x, y int) {
	t.pressed = false
}

func (t *Input) clampCursor(x int) int {
	if x < 0 {
		return 0
	}
	if x > len(t.text) {
		return len(t.text)
	}
	return x
}

func (t *Input) Select(start, end int) {
	t.anchor = start
	t.cursor = end
}

// Returns the normalized selection range (start <= end)
func (t *Input) selection() (int, int, bool) {
	if t.anchor == t.cursor {
		return 0, 0, false
	}
	start, end := t.anchor, t.cursor
	if start > end {
		start, end = end, start
	}
	return start, end, true
}

// List displays a vertical list of items.
// The zero value is ready to use.
type List struct {
	Items    []ListItem
	Index    int // current selected index, -1 means none
	OnSelect func(ListItem)
}

type ListItem struct {
	Name  string
	Value any
}

func (l *List) Size() (int, int) {
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

func (l *List) Draw(s Screen, rect Rect) {
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
		DrawString(s, rect.X, rect.Y+i, rect.W, label, st)
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

func (l *List) OnFocus() {}
func (l *List) OnBlur()  {}

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

func (d *Divider) Size() (w, h int) { return 1, 1 }
func (d *Divider) Layout(r Rect) *Node {
	return &Node{
		Element: d,
		Rect:    r,
	}
}
func (d *Divider) Draw(s Screen, rect Rect) {
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

func (e empty) Size() (int, int) { return 0, 0 }
func (e empty) Layout(r Rect) *Node {
	return &Node{Element: e, Rect: r}
}
func (e empty) Draw(Screen, Rect) {}
