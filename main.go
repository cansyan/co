package main

import (
	"flag"
	"fmt"
	"go/format"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cansyan/co/ui"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

var verbose = flag.Bool("v", false, "enable verbose logging")

func main() {
	flag.Parse()

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		path := filepath.Join(os.TempDir(), "co.log")
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		log.SetOutput(f)
		ui.Logger = log.Default()
	} else {
		log.SetOutput(io.Discard)
	}

	app := ui.NewApp()
	app.BindKey("Ctrl+Q", app.Stop)

	editorApp := newEditorApp(app)
	if arg := flag.Arg(0); arg != "" {
		path, line := parseFileArg(arg)
		err := editorApp.openFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		if line > 0 {
			if e := editorApp.getEditor(); e != nil {
				e.gotoLine(line)
			}
		}
	} else {
		editorApp.newTab("untitled")
	}
	editorApp.requestFocus()

	if err := app.Run(editorApp); err != nil {
		log.Print(err)
		return
	}
}

func parseFileArg(arg string) (path string, line int) {
	parts := strings.Split(arg, ":")
	path = parts[0]
	if len(parts) > 1 {
		line, _ = strconv.Atoi(parts[1])
	}
	return path, line
}

var _ ui.Focusable = (*EditorApp)(nil)

type EditorApp struct {
	app       *ui.App
	tabs      []*tab
	activeTab int
	newBtn    *ui.Button
	saveBtn   *ui.Button
	quitBtn   *ui.Button
	backBtn   *ui.Button
	fwdBtn    *ui.Button
	status    string

	searchBar  *SearchBar
	showSearch bool
	clipboard  string

	leaderKeyActive bool
	leaderTimer     *time.Timer

	history        []historyEntry
	historyPos     int
	navigatingHist bool
}

type historyEntry struct {
	path string
	pos  ui.Pos
}

func newEditorApp(uiApp *ui.App) *EditorApp {
	r := &EditorApp{app: uiApp, historyPos: -1}
	r.newBtn = &ui.Button{
		Text: "＋",
		OnClick: func() {
			r.newTab("untitled")
			uiApp.SetFocus(r)
		},
	}

	r.backBtn = &ui.Button{Text: "←", OnClick: r.goBack}
	r.fwdBtn = &ui.Button{Text: "→", OnClick: r.goForward}
	r.saveBtn = &ui.Button{Text: "Save", OnClick: r.saveFile}
	r.quitBtn = &ui.Button{Text: "Quit", OnClick: uiApp.Stop}
	r.searchBar = NewSearchBar(r)
	return r
}

func (a *EditorApp) newTab(label string) {
	a.tabs = append(a.tabs, newTab(a, label))
	a.activeTab = len(a.tabs) - 1
}

// closeTab closes the tab at index i, prompting to save if there are unsaved changes.
func (a *EditorApp) closeTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	tab := a.tabs[i]
	editor := tab.editor
	if !editor.Dirty {
		a.deleteTab(i)
		a.requestFocus()
		return
	}

	saveBtn := &ui.Button{
		Text: "Save",
		OnClick: func() {
			if path := tab.path; path != "untitled" {
				if err := a.writeFile(path, editor); err != nil {
					log.Print(err)
					a.setStatus(err.Error(), 5*time.Second)
					return
				}
				a.deleteTab(i)
				a.app.CloseOverlay()
				a.requestFocus()
				return
			}

			a.promptSaveAs(func(path string) {
				if path == "" {
					return
				}
				if err := a.writeFile(path, editor); err != nil {
					log.Print(err)
					a.setStatus(err.Error(), 5*time.Second)
					return
				}
				a.deleteTab(i)
				a.requestFocus()
			})
		},
		Style: ui.Style{BG: ui.Theme.Selection},
	}

	// Prompt to save changes.
	view := ui.Border(ui.VStack(
		ui.PadH(ui.NewText("Save the changes before closing?"), 1),
		ui.PadH(ui.HStack(
			ui.NewButton("Don't Save", func() {
				a.deleteTab(i)
				a.app.CloseOverlay()
				a.requestFocus()
			}),
			ui.PadH(ui.NewButton("Cancel", a.app.CloseOverlay), 2),
			saveBtn,
		), 2),
	).Spacing(1))
	a.app.Overlay(view, "top")
}

func (a *EditorApp) deleteTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}

	a.tabs = slices.Delete(a.tabs, i, i+1)
	if i < a.activeTab {
		a.activeTab--
	} else if i == a.activeTab {
		a.activeTab = max(0, len(a.tabs)-1)
	}

	if len(a.tabs) == 0 {
		a.app.Stop()
	}
}

func (a *EditorApp) MinSize() (int, int) {
	var maxW, maxH int
	for _, t := range a.tabs {
		w, h := t.editor.MinSize()
		if w > maxW {
			maxW = w
		}
		if h > maxH {
			maxH = h
		}
	}
	return maxW, maxH + 1 // +1 for tab label
}

func (a *EditorApp) Layout(x, y, w, h int) *ui.LayoutNode {
	tabLabels := ui.HStack()
	for i, tab := range a.tabs {
		tabLabels.Append(tab)
		if i != len(a.tabs)-1 {
			tabLabels.Append(&ui.Divider{})
		}
	}

	mainStack := ui.VStack()
	mainStack.Append(
		ui.HStack(ui.Grow(tabLabels), a.backBtn, a.fwdBtn, a.newBtn, a.saveBtn, a.quitBtn),
	)
	if len(a.tabs) > 0 {
		mainStack.Append(ui.Grow(a.tabs[a.activeTab].editor))
	}
	if a.showSearch {
		mainStack.Append(&ui.Divider{}, a.searchBar)
	}

	statusBar := ui.HStack()
	if e := a.getEditor(); e != nil {
		posInfo := fmt.Sprintf("Line %d, Column %d", e.Pos.Row+1, e.Pos.Col+1)
		statusBar.Append(ui.NewText(posInfo))
	}
	if a.status != "" {
		statusBar.Append(ui.Spacer, ui.NewText(a.status))
	}
	if a.leaderKeyActive {
		statusBar.Append(ui.Spacer, ui.NewText("Wait for key..."))
	}

	mainStack.Append(
		&ui.Divider{},
		ui.PadH(statusBar, 1),
	)

	n := ui.NewLayoutNode(a, x, y, w, h)
	n.Children = []*ui.LayoutNode{mainStack.Layout(x, y, w, h)}
	return n
}

// setStatus sets the status text and clears it after a delay.
func (a *EditorApp) setStatus(msg string, delay time.Duration) {
	a.status = msg
	time.AfterFunc(delay, func() {
		// Only clear if the current message is still the one we set.
		// (Avoid clearing a newer, different message).
		if a.status == msg {
			a.status = ""
			a.app.Refresh()
		}
	})
}

func (a *EditorApp) Render(ui.Screen, ui.Rect) {
	// no-op
}

// a convenient method to request focus back to the app.
func (a *EditorApp) requestFocus() {
	a.app.SetFocus(a)
}

// delegates focus to the active tab's editor.
func (a *EditorApp) FocusTarget() ui.Element {
	if len(a.tabs) == 0 {
		return a
	}
	return a.tabs[a.activeTab].editor
}

func (a *EditorApp) OnFocus() {}
func (a *EditorApp) OnBlur()  {}

func (a *EditorApp) resetFind() {
	a.showSearch = true
	sb := a.searchBar
	sb.matches = nil
	sb.activeIndex = -1

	// reuse previous query or selected text, but select all for easy replacement
	query := sb.input.Text()
	if e := a.getEditor(); e != nil {
		if s := e.SelectedText(); s != "" {
			query = s
			sb.input.SetText(s)
		}
	}
	sb.input.Select(0, len([]rune(query)))

	a.app.SetFocus(sb)
}

func (a *EditorApp) openFileDialog() {
	input := &ui.TextInput{
		Placeholder: "Open file path: ",
		OnCommit: func(text string) {
			if text != "" {
				if err := a.openFile(text); err != nil {
					log.Print(err)
					a.setStatus(err.Error(), 3*time.Second)
				}
			}
			a.app.CloseOverlay()
			a.requestFocus()
		},
	}
	view := ui.Border(ui.Frame(input, 60, 1))
	a.app.Overlay(view, "top")
}

// handles app-level commands
func (a *EditorApp) handleGlobalKey(ev *tcell.EventKey) bool {
	switch strings.ToLower(ev.Name()) {
	case "ctrl+s":
		a.saveFile()
		return true
	case "ctrl+w":
		a.closeTab(a.activeTab)
		return true
	case "ctrl+f":
		a.resetFind()
		return true
	case "ctrl+k":
		a.activateLeader()
		return true
	case "ctrl+p":
		if a.leaderKeyActive {
			a.leaderKeyActive = false
			if a.leaderTimer != nil {
				a.leaderTimer.Stop()
			}
			a.showPalette(">") // Ctrl+K Ctrl+P command mode
		} else {
			a.showPalette("") // default file search mode
		}
		return true
	case "ctrl+r":
		a.showPalette("@")
		return true
	case "ctrl+o":
		a.openFileDialog()
		return true
	case "ctrl+t":
		a.newTab("untitled")
		a.requestFocus()
		return true
	case "esc":
		if a.showSearch {
			a.showSearch = false
			a.requestFocus()
			return true
		}
		return true
	}
	return false
}

func (a *EditorApp) getEditor() *Editor {
	if len(a.tabs) == 0 {
		return nil
	}
	return a.tabs[a.activeTab].editor
}

func (a *EditorApp) showPalette(prefix string) {
	p := NewPalette()
	p.input.OnChange = func() {
		text := p.input.Text()
		p.list.Clear()
		p.list.Selected = 0

		switch {
		case strings.HasPrefix(text, ":"):
			// 1. Go to Line
			lineStr := text[1:]
			n, err := strconv.Atoi(lineStr)
			if err != nil || n < 1 {
				return
			}

			p.list.Append(fmt.Sprintf("Go to Line %d", n), func() {
				if e := a.getEditor(); e != nil {
					e.gotoLine(n)
				}
				a.requestFocus()
			})

		case strings.HasPrefix(text, "@"):
			// 2. Go to Symbol
			query := strings.ToLower(text[1:])
			// Split by spaces or dots
			words := strings.FieldsFunc(query, func(r rune) bool {
				return r == ' ' || r == '.'
			})
			editor := a.getEditor()
			if editor == nil {
				return
			}

			for _, s := range editor.symbols {
				ok := true
				for _, word := range words {
					if word == "" {
						continue
					}
					if !strings.Contains(strings.ToLower(s.Signature), word) {
						ok = false
						break
					}
				}
				if ok {
					p.list.Append(s.Signature, func() {
						editor.gotoLine(s.Line + 1)
						a.requestFocus()
					})
				}
			}

		case strings.HasPrefix(text, ">"):
			// 3. Command Mode
			a.fillCommandMode(p, text[1:])
		default:
			// 4. File Search Mode (Default)
			a.fillFileSearchMode(p, text)
		}
	}

	p.input.SetText(prefix)
	a.app.Overlay(p, "top")
}

func (a *EditorApp) fillCommandMode(p *Palette, query string) {
	words := strings.Fields(query)
	commands := []struct {
		name   string
		action func()
	}{
		{"Color Theme: Breaks", func() { ui.Theme = ui.NewBreakersTheme() }},
		{"Color Theme: Mariana", func() { ui.Theme = ui.NewMarianaTheme() }},
		{"Goto Definition", func() {
			if e := a.getEditor(); e != nil {
				e.gotoDefinition()
			}
		}},
		{"Goto Symbol", func() { a.showPalette("@") }},
		{"New File", func() { a.newTab("untitled"); a.requestFocus() }},
		{"Quit", a.app.Stop},
	}
	for _, cmd := range commands {
		ok := true
		for _, word := range words {
			if word == "" {
				continue
			}
			if !strings.Contains(strings.ToLower(cmd.name), word) {
				ok = false
				break
			}
		}
		if ok {
			p.list.Append(cmd.name, func() {
				cmd.action()
				a.requestFocus()
			})
		}
	}
}

func (a *EditorApp) fillFileSearchMode(p *Palette, query string) {
	query = strings.ToLower(query)
	filter := make(map[string]bool)
	currentDir, _ := os.Getwd()

	// list opened tabs first
	for i, t := range a.tabs {
		path := t.path
		// show relative path if possible
		if filepath.IsAbs(t.path) {
			if rel, err := filepath.Rel(currentDir, t.path); err == nil && !strings.HasPrefix(rel, "..") {
				path = rel
			}
		}

		if query == "" || strings.Contains(strings.ToLower(path), query) {
			p.list.Append(path, func() {
				a.activeTab = i
				a.requestFocus()
			})
			filter[path] = true
			if p.list.Len() >= 10 {
				return
			}
		}
	}

	// list files in current directory
	entries, _ := os.ReadDir(".")
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filter[name] || strings.HasPrefix(name, ".") {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(name), query) {
			continue
		}
		p.list.Append(name, func() {
			a.openFile(name)
			a.requestFocus()
		})
		if p.list.Len() >= 10 {
			return
		}
	}
}

func (a *EditorApp) openFile(name string) error {
	abs, err := filepath.Abs(name)
	if err != nil {
		return err
	}

	// tab existed, just switch
	for i, tab := range a.tabs {
		if tab.path == abs {
			a.activeTab = i
			a.recordJump()
			return nil
		}
	}

	bs, err := os.ReadFile(abs)
	if err != nil {
		return err
	}

	a.newTab(abs)
	editor := a.getEditor()
	editor.SetText(string(bs))
	editor.updateSymbols()
	a.recordJump()
	return nil
}

// pushHistory tracks significant cursor movements
// (like jumping to definitions, jumping between files, large jumps within a file),
// not every single cursor movement
func (a *EditorApp) pushHistory(path string, pos ui.Pos) {
	if a.navigatingHist {
		return
	}
	entry := historyEntry{path: path, pos: pos}
	// truncate forward history when pushing new entry
	if a.historyPos >= 0 && a.historyPos < len(a.history)-1 {
		a.history = a.history[:a.historyPos+1]
	}
	// avoid duplicate consecutive entries with same location
	if len(a.history) > 0 {
		last := a.history[len(a.history)-1]
		if last.path == path && last.pos.Row == pos.Row {
			return
		}
	}
	a.history = append(a.history, entry)
	a.historyPos = len(a.history) - 1
}

func (a *EditorApp) recordJump() {
	if e := a.getEditor(); e != nil && a.tabs[a.activeTab].path != "" {
		a.pushHistory(a.tabs[a.activeTab].path, e.Pos)
	}
}

func (a *EditorApp) goBack() {
	defer a.requestFocus()
	if a.historyPos <= 0 {
		return
	}
	a.historyPos--
	a.navigateToHistory()
}

func (a *EditorApp) goForward() {
	defer a.requestFocus()
	if a.historyPos >= len(a.history)-1 {
		return
	}
	a.historyPos++
	a.navigateToHistory()
}

func (a *EditorApp) navigateToHistory() {
	if a.historyPos < 0 || a.historyPos >= len(a.history) {
		return
	}
	a.navigatingHist = true
	defer func() { a.navigatingHist = false }()

	entry := a.history[a.historyPos]
	if err := a.openFile(entry.path); err != nil {
		log.Print(err)
		a.setStatus(err.Error(), 3*time.Second)
		return
	}
	if e := a.getEditor(); e != nil {
		e.SetCursor(entry.pos.Row, entry.pos.Col)
		e.CenterRow(entry.pos.Row)
	}
}

func (a *EditorApp) saveFile() {
	if len(a.tabs) == 0 {
		return
	}
	tab := a.tabs[a.activeTab]
	editor := tab.editor
	if !editor.Dirty {
		a.requestFocus()
		return
	}

	if path := tab.path; path != "untitled" {
		if err := a.writeFile(path, editor); err != nil {
			log.Print(err)
			a.setStatus(err.Error(), 5*time.Second)
			return
		}
		a.setStatus("Saved "+path, 2*time.Second)
		a.requestFocus()
		return
	}

	a.promptSaveAs(func(path string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			log.Print(err)
			a.setStatus(err.Error(), 5*time.Second)
			return
		}
		if err := a.writeFile(abs, editor); err != nil {
			log.Print(err)
			a.setStatus(err.Error(), 5*time.Second)
			return
		}
		tab.path = abs
		a.requestFocus()
	})
}

func (a *EditorApp) promptSaveAs(commit func(path string)) {
	input := &ui.TextInput{
		OnCommit: func(text string) {
			if commit != nil {
				commit(text)
			}
			a.app.CloseOverlay()
		},
	}

	okBtn := &ui.Button{
		Text: "OK",
		OnClick: func() {
			if commit != nil {
				commit(input.Text())
			}
			a.app.CloseOverlay()
		},
		Style: ui.Style{BG: ui.Theme.Selection},
	}

	dialog := ui.Frame(
		ui.Border(ui.VStack(
			ui.PadH(ui.HStack(
				ui.NewText("Save as: "),
				ui.Grow(input),
			), 1),

			ui.PadH(ui.HStack(
				ui.NewButton("Cancel", a.app.CloseOverlay),
				ui.Spacer,
				okBtn,
			), 4),
		).Spacing(1)),
		40, 0,
	)
	a.app.Overlay(dialog, "top")
	a.app.SetFocus(input)
}

func (a *EditorApp) writeFile(path string, e *Editor) error {
	bs := []byte(e.String())

	// Only format if it's a Go file
	if filepath.Ext(path) == ".go" {
		formatted, err := format.Source(bs)
		if err == nil {
			bs = formatted
			row, col := e.Pos.Row, e.Pos.Col
			e.SetText(string(formatted)) // Sync formatted text back to UI
			e.SetCursor(row, col)
		} else {
			// If formatting fails (e.g., syntax error), we still save
			// but notify the user via status bar.
			a.setStatus(fmt.Sprintf("Format error: %v", err), 5*time.Second)
		}
	}

	err := os.WriteFile(path, bs, 0644)
	if err != nil {
		return err
	}

	e.Dirty = false
	e.updateSymbols()
	return nil
}

type tab struct {
	a        *EditorApp
	path     string
	closeBtn *ui.Button
	editor   *Editor
	hovered  bool
}

func newTab(root *EditorApp, label string) *tab {
	t := &tab{
		a:    root,
		path: label,
	}
	t.closeBtn = ui.NewButton("✕", func() {
		for i, tab := range root.tabs {
			if tab == t {
				t.a.closeTab(i)
				return
			}
		}
	})
	e := NewEditor(root)
	ext := filepath.Ext(label)
	switch ext {
	case ".go":
		e.SetHighlighter(highlightGo)
	case ".md", ".markdown":
		e.SetHighlighter(highlightMarkdown)
	}
	t.editor = e
	return t
}

const tabItemWidth = 18

func (t *tab) MinSize() (int, int) { return tabItemWidth, 1 }
func (t *tab) Layout(x, y, w, h int) *ui.LayoutNode {
	bw, bh := t.closeBtn.MinSize()
	return &ui.LayoutNode{
		Element: t,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
		Children: []*ui.LayoutNode{
			t.closeBtn.Layout(x+tabItemWidth-3, y, bw, bh),
		},
	}
}
func (t *tab) Render(screen ui.Screen, r ui.Rect) {
	var style ui.Style
	if t == t.a.tabs[t.a.activeTab] {
		style.FontUnderline = true
	} else if t.hovered {
		style.BG = ui.Theme.Hover
	}

	labelWidth := tabItemWidth - 3 - 1 // minus button and padding
	label := filepath.Base(t.path)
	if runewidth.StringWidth(label) <= labelWidth {
		label = runewidth.FillRight(label, labelWidth)
	} else {
		label = runewidth.Truncate(label, labelWidth, "…")
	}
	label = fmt.Sprintf(" %s", label)
	ui.DrawString(screen, r.X, r.Y, r.W, label, style.Apply())
}

func (t *tab) OnMouseDown(lx, ly int) {
	// like Sublime Text, instant react on clicking tab, not waiting the mouse up
	for i, tab := range t.a.tabs {
		if tab == t {
			t.a.activeTab = i
			t.a.app.SetFocus(t.a)
			return
		}
	}
}

func (t *tab) OnMouseUp(lx, ly int) {}
func (t *tab) OnMouseEnter() {
	t.hovered = true
}
func (t *tab) OnMouseLeave() {
	t.hovered = false
}
func (t *tab) OnMouseMove(rx, ry int) {}

type Palette struct {
	input *ui.TextInput
	list  *ui.ListView
}

func NewPalette() *Palette {
	p := &Palette{
		input: new(ui.TextInput),
		list:  ui.NewListView(),
	}
	return p
}

func (p *Palette) SetText(text string) {
	p.input.SetText(text)
	p.input.OnFocus()
}

func (p *Palette) MinSize() (int, int) {
	w1, h1 := 60, 1 // input box size
	// avoid full screen list items
	_, h2 := p.list.MinSize()
	if h2 > 15 {
		h2 = 15
	}
	return w1 + 2, h1 + h2 + 2 // +2 for the border
}

func (p *Palette) Layout(x, y, w, h int) *ui.LayoutNode {
	n := &ui.LayoutNode{
		Element: p,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
	view := ui.Border(ui.VStack(
		p.input,
		p.list,
	))
	n.Children = append(n.Children, view.Layout(x, y, w, h))
	return n
}

func (p *Palette) Render(ui.Screen, ui.Rect) {
	// no-op
}

func (p *Palette) HandleKey(ev *tcell.EventKey) bool {
	consumed := true
	switch ev.Key() {
	case tcell.KeyDown, tcell.KeyCtrlN:
		p.list.SelectNext()
	case tcell.KeyUp, tcell.KeyCtrlP:
		p.list.SelectPrev()
	case tcell.KeyEnter:
		p.list.Activate()
	default:
		p.input.HandleKey(ev)
		consumed = false
	}
	return consumed
}

func (p *Palette) OnFocus() { p.input.OnFocus() }
func (p *Palette) OnBlur()  { p.input.OnBlur() }

type symbol struct {
	Name      string // identifier, for example "saveFile"
	Signature string // readable definition, for example "(*root).saveFile"
	Line      int    // line number
	Kind      string // "type", "func" (for function and method)
}

func extractSymbols(content string) []symbol {
	var symbols []symbol
	// Group 1: Receiver (optional), Group 2: Name
	funcRegex := regexp.MustCompile(`(?m)^func\s+(?:\(([^)]+)\)\s+)?([a-zA-Z_]\w*)`)
	typeRegex := regexp.MustCompile(`(?m)^type\s+([a-zA-Z_]\w*)`)

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// 1. 處理函式與方法
		if matches := funcRegex.FindStringSubmatch(line); len(matches) > 0 {
			rawRecv := matches[1]
			name := matches[2]

			sign := name
			if rawRecv != "" {
				// 提取 Receiver 型別並組合成 (*root).Method 格式
				parts := strings.Fields(rawRecv)
				recvType := parts[len(parts)-1]
				sign = fmt.Sprintf("(%s).%s", recvType, name)
			}

			symbols = append(symbols, symbol{
				Name:      name,
				Signature: sign,
				Line:      i,
				Kind:      "func",
			})
			continue
		}

		// 2. 處理型別定義
		if matches := typeRegex.FindStringSubmatch(line); len(matches) > 1 {
			name := matches[1]
			symbols = append(symbols, symbol{
				Name:      name,
				Signature: "type " + name,
				Line:      i,
				Kind:      "type",
			})
		}
	}
	return symbols
}

type SearchBar struct {
	a           *EditorApp
	input       *proxyInput
	btnPrev     *ui.Button
	btnNext     *ui.Button
	closeBtn    *ui.Button
	matches     []ui.Pos
	activeIndex int // -1 表示尚未進行導航定位
}

func NewSearchBar(r *EditorApp) *SearchBar {
	sb := &SearchBar{a: r, activeIndex: -1}
	sb.input = &proxyInput{
		TextInput: new(ui.TextInput),
		parent:    sb,
	}
	// Lazy Evaluation:
	// 當文字改變時，僅標記狀態為「需要重新掃描」，但不立即掃描
	// 真正的計算成本被推遲到了使用者按下 Enter、SelectNext 或 SelectPrev 的那一刻
	sb.input.OnChange = func() {
		sb.matches = nil
		sb.activeIndex = -1
	}

	sb.btnPrev = ui.NewButton("↑", func() { sb.navigate(false) })
	sb.btnNext = ui.NewButton("↓", func() { sb.navigate(true) })
	sb.closeBtn = ui.NewButton("✕", func() {
		sb.a.showSearch = false
		sb.a.requestFocus()
	})
	return sb
}

func (sb *SearchBar) updateMatches() {
	sb.matches = nil
	sb.activeIndex = -1

	query := strings.ToLower(sb.input.Text())
	if query == "" {
		return
	}

	editor := sb.a.getEditor()
	if editor == nil {
		return
	}

	content := editor.String()
	lowerContent := strings.ToLower(content)

	currentPos := 0
	lineCount := 0
	lastLineStart := 0

	for {
		idx := strings.Index(lowerContent[currentPos:], query)
		if idx == -1 {
			break
		}

		// 絕對座標
		matchPos := currentPos + idx

		// 計算從上一次匹配到現在經過了多少個換行符
		// 這樣只需要掃描匹配點之間的區間
		for i := currentPos; i < matchPos; i++ {
			if content[i] == '\n' {
				lineCount++
				lastLineStart = i + 1
			}
		}

		// 處理中文寬度：將 Byte 偏移量轉為 Rune 偏移量
		// 只需要計算從該行起始 (lastLineStart) 到 匹配點 (matchPos) 之間有多少個 UTF-8 Rune
		linePrefix := content[lastLineStart:matchPos]
		runeCol := utf8.RuneCountInString(linePrefix)

		sb.matches = append(sb.matches, ui.Pos{
			Row: lineCount,
			Col: runeCol,
		})

		currentPos = matchPos + len(query)
	}
}

// 根據編輯器當前游標位置，找到最接近的匹配項索引
func (sb *SearchBar) setInitialActiveIndex() {
	if len(sb.matches) == 0 {
		return
	}

	tab := sb.a.tabs[sb.a.activeTab]
	e := tab.editor

	// 尋找第一個在游標位置之後的匹配項
	for i, m := range sb.matches {
		if m.Row > e.Pos.Row || (m.Row == e.Pos.Row && m.Col >= e.Pos.Col) {
			sb.activeIndex = i
			return
		}
	}

	// 若游標已在所有匹配項之後，則循環回第一個
	sb.activeIndex = 0
}

func (sb *SearchBar) navigate(forward bool) {
	// 只有在真正需要結果時才更新 matches
	if sb.matches == nil {
		sb.updateMatches()
	}

	count := len(sb.matches)
	if count == 0 {
		return
	}

	// 首次導航：尋找最接近游標的匹配項
	if sb.activeIndex == -1 {
		sb.setInitialActiveIndex()
		// 如果是向上找(Prev)，在定位後需再往前退一格
		if !forward {
			sb.activeIndex = (sb.activeIndex - 1 + count) % count
		}
	} else {
		// 常規移動
		if forward {
			sb.activeIndex = (sb.activeIndex + 1) % count
		} else {
			sb.activeIndex = (sb.activeIndex - 1 + count) % count
		}
	}

	sb.syncEditor()
}

func (sb *SearchBar) syncEditor() {
	m := sb.matches[sb.activeIndex]
	editor := sb.a.getEditor()
	if editor == nil {
		return
	}
	queryLen := utf8.RuneCountInString(sb.input.Text())
	editor.CenterRow(m.Row)
	editor.SetSelection(m, ui.Pos{Row: m.Row, Col: m.Col + queryLen})
}

func (sb *SearchBar) Layout(x, y, w, h int) *ui.LayoutNode {
	countStr := " 0/0 "
	if len(sb.matches) > 0 {
		displayIdx := sb.activeIndex + 1
		if sb.activeIndex == -1 {
			displayIdx = 0
		}
		countStr = fmt.Sprintf(" %d/%d ", displayIdx, len(sb.matches))
	}

	node := ui.NewLayoutNode(sb, x, y, w, h)
	view := ui.HStack(
		ui.PadH(ui.NewText("Find:"), 1),
		ui.Grow(sb.input),
		ui.PadH(ui.NewText(countStr), 1),
		sb.btnPrev,
		sb.btnNext,
		sb.closeBtn,
	)
	node.Children = []*ui.LayoutNode{view.Layout(x, y, w, h)}
	return node
}

func (sb *SearchBar) MinSize() (int, int) {
	return 10, 1
}

func (sb *SearchBar) Render(s ui.Screen, r ui.Rect) {}

func (sb *SearchBar) HandleKey(ev *tcell.EventKey) bool {
	consumed := true
	switch ev.Key() {
	case tcell.KeyEnter:
		sb.navigate(true)
	case tcell.KeyUp, tcell.KeyCtrlP:
		sb.navigate(false)
	case tcell.KeyDown, tcell.KeyCtrlN:
		sb.navigate(true)
	case tcell.KeyESC:
		sb.a.showSearch = false
		sb.a.app.SetFocus(sb.a)
	default:
		sb.input.HandleKey(ev)
		consumed = false
	}
	return consumed
}

func (sb *SearchBar) OnFocus() {
	// make TextInput show cursor
	sb.input.OnFocus()
}
func (sb *SearchBar) OnBlur() { sb.input.OnBlur() }

// proxy TextInput, can redirect the focus to parent.
type proxyInput struct {
	*ui.TextInput
	parent ui.Element
}

func (p *proxyInput) Layout(x, y, w, h int) *ui.LayoutNode {
	return &ui.LayoutNode{
		Element: p,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
}

func (p *proxyInput) FocusTarget() ui.Element {
	return p.parent
}

func (a *EditorApp) activateLeader() {
	a.leaderKeyActive = true
	if a.leaderTimer != nil {
		a.leaderTimer.Stop()
	}
	// 2 秒後自動重置狀態
	a.leaderTimer = time.AfterFunc(2*time.Second, func() {
		a.leaderKeyActive = false
		a.app.Refresh()
	})
}

type Editor struct {
	*ui.TextEditor
	app     *EditorApp
	symbols []symbol
}

func NewEditor(r *EditorApp) *Editor {
	e := &Editor{
		TextEditor: ui.NewTextEditor(),
		app:        r,
	}

	e.InlineSuggest = true
	// For now, the suggester just suggests the first matching symbol name.
	// It can extend to a more advanced one later: multiple results, time constraint,
	// ignore case, fuzzy match, append () for function, etc.
	e.Suggester = func(prefix string) string {
		for _, s := range e.symbols {
			if len(s.Name) > len(prefix) && strings.HasPrefix(s.Name, prefix) {
				if s.Kind == "func" {
					return s.Name[len(prefix):] + "()"
				}
				return s.Name[len(prefix):]
			}
		}
		return ""
	}
	return e
}

func (e *Editor) Layout(x, y, w, h int) *ui.LayoutNode {
	return &ui.LayoutNode{
		Element: e,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
}

// HandleKey handles editor-specific keybindings.
// If the key is not handled here, it will bubble up to the app level.
func (e *Editor) HandleKey(ev *tcell.EventKey) bool {
	switch strings.ToLower(ev.Name()) {
	case "ctrl+z":
		e.TextEditor.Undo()
		return true
	case "ctrl+y":
		e.TextEditor.Redo()
		return true
	case "ctrl+a":
		lastLine := e.Line(e.Len() - 1)
		e.SetSelection(ui.Pos{}, ui.Pos{Row: e.Len() - 1, Col: len(lastLine)})
	case "ctrl+c":
		s := e.SelectedText()
		if s == "" {
			// copy current line by default
			e.app.clipboard = string(e.Line(e.Pos.Row))
			return true
		}
		e.app.clipboard = s
	case "ctrl+x":
		e.TextEditor.SaveEdit()
		e.MergeNext = false
		start, end, ok := e.Selection()
		if !ok {
			// cut line by default
			e.app.clipboard = string(e.Line(e.Pos.Row)) + "\n"
			e.DeleteRange(ui.Pos{Row: e.Pos.Row}, ui.Pos{Row: e.Pos.Row + 1})
			return true
		}

		e.app.clipboard = e.SelectedText()
		e.DeleteRange(start, end)
		e.ClearSelection()
	case "ctrl+v":
		e.TextEditor.SaveEdit()
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
		e.gotoLine(1)
	case "alt+down": // goto last line
		e.gotoLine(e.Len())
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
		if !e.TextEditor.HandleKey(ev) {
			// Bubble event to parent
			return e.app.handleGlobalKey(ev)
		}
	}
	return true
}

func (e *Editor) gotoLine(line int) {
	if line < 1 {
		line = 1
	}
	if line > e.Len() {
		line = e.Len()
	}
	e.SetCursor(line-1, 0)
	e.CenterRow(line - 1)
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
			e.gotoLine(s.Line + 1)
			return
		}
	}
}

func (e *Editor) updateSymbols() {
	e.symbols = extractSymbols(e.String())
}

func (e *Editor) OnMouseDown(lx, ly int) {
	e.TextEditor.OnMouseDown(lx, ly)
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
				}
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

func highlightMarkdown(line []rune) []ui.StyleSpan {
	var spans []ui.StyleSpan
	if len(line) == 0 {
		return spans
	}

	// Headers: # ## ### etc.
	if line[0] == '#' {
		spans = append(spans, ui.StyleSpan{
			Start: 0,
			End:   len(line),
			Style: ui.Theme.Syntax.Keyword,
		})
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
					Style: ui.Theme.Syntax.Keyword,
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
					Style: ui.Theme.Syntax.String,
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
