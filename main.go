package main

import (
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
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

	if detectLightTerminal() {
		ui.Theme = ui.Breakers
	}

	manager := ui.NewManager()
	manager.BindKey("Ctrl+Q", manager.Stop)

	// Handle signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		manager.Stop()
	}()

	app := newApp(manager)
	if arg := flag.Arg(0); arg != "" {
		path, line := parseFileArg(arg)
		err := app.openFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		if line > 0 {
			if e := app.getEditor(); e != nil {
				e.gotoLine(line - 1)
			}
		}
	} else {
		app.newTab("untitled")
	}
	app.requestFocus()

	if err := manager.Start(app); err != nil {
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

var _ ui.Focusable = (*App)(nil)

type App struct {
	manager   *ui.Manager
	tabs      []*tab
	activeTab int
	newBtn    *ui.Button
	openBtn   *ui.Button
	saveBtn   *ui.Button
	quitBtn   *ui.Button
	backBtn   *ui.Button
	fwdBtn    *ui.Button
	status    string

	searchBar  *SearchBar
	showSearch bool
	clipboard  string // local cache for immediate paste; also synced to OS clipboard

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

func newApp(m *ui.Manager) *App {
	a := &App{
		manager:    m,
		historyPos: -1,
	}
	a.newBtn = &ui.Button{
		Text: "New",
		OnClick: func() {
			a.newTab("untitled")
			m.SetFocus(a)
		},
	}
	a.openBtn = &ui.Button{Text: "Open", OnClick: a.promptOpen}
	a.backBtn = &ui.Button{Text: "←", OnClick: a.goBack}
	a.fwdBtn = &ui.Button{Text: "→", OnClick: a.goForward}
	a.saveBtn = &ui.Button{Text: "Save", OnClick: a.saveFile}
	a.quitBtn = &ui.Button{Text: "Quit", OnClick: m.Stop}
	a.searchBar = NewSearchBar(a)
	return a
}

func (a *App) newTab(label string) {
	a.tabs = append(a.tabs, newTab(a, label))
	a.activeTab = len(a.tabs) - 1
}

// closeTab closes the tab at index i, prompting to save if there are unsaved changes.
func (a *App) closeTab(i int) {
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
				a.manager.CloseOverlay()
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
				a.manager.CloseOverlay()
				a.requestFocus()
			}),
			ui.PadH(ui.NewButton("Cancel", a.manager.CloseOverlay), 2),
			saveBtn,
		), 2),
	).Spacing(1))
	a.manager.Overlay(view, "top")
}

func (a *App) deleteTab(i int) {
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
		a.manager.Stop()
	}
}

func (a *App) Size() (int, int) {
	var maxW, maxH int
	for _, t := range a.tabs {
		w, h := t.editor.Size()
		if w > maxW {
			maxW = w
		}
		if h > maxH {
			maxH = h
		}
	}
	return maxW, maxH + 1 // +1 for tab label
}

func (a *App) Layout(r ui.Rect) *ui.Node {
	// It is not efficient to recreate the UI components on every layout call,
	// but for now it is acceptable given the simplicity of the app.
	tabLabels := ui.HStack()
	for i, tab := range a.tabs {
		tabLabels.Append(tab)
		if i != len(a.tabs)-1 {
			tabLabels.Append(&ui.Divider{})
		}
	}

	mainStack := ui.VStack()
	mainStack.Append(
		ui.HStack(ui.Grow(tabLabels), a.backBtn, a.fwdBtn, a.newBtn, a.openBtn, a.saveBtn, a.quitBtn),
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

	return &ui.Node{
		Element:  a,
		Rect:     r,
		Children: []*ui.Node{mainStack.Layout(r)},
	}
}

// setStatus sets the status text and clears it after a delay.
func (a *App) setStatus(msg string, delay time.Duration) {
	a.status = msg
	time.AfterFunc(delay, func() {
		// Only clear if the current message is still the one we set.
		// (Avoid clearing a newer, different message).
		if a.status == msg {
			a.status = ""
			a.manager.Refresh()
		}
	})
}

func (a *App) Draw(ui.Screen, ui.Rect) {
	// no-op
}

// a convenient method to request focus back to the app.
func (a *App) requestFocus() {
	a.manager.SetFocus(a)
}

// delegates focus to the active tab's editor.
func (a *App) FocusTarget() ui.Element {
	if len(a.tabs) == 0 {
		return a
	}
	return a.tabs[a.activeTab].editor
}

func (a *App) OnFocus() {}
func (a *App) OnBlur()  {}

func (a *App) resetFind() {
	a.showSearch = true
	sb := a.searchBar
	sb.matches = nil
	sb.activeIndex = -1

	// reuse previous query or selected text, but select all for easy replacement
	query := sb.input.String()
	if e := a.getEditor(); e != nil {
		if s := e.SelectedText(); s != "" {
			query = s
			sb.input.SetText(s)
		}
	}
	sb.input.Select(0, len([]rune(query)))

	a.manager.SetFocus(sb)
}

// handles app-level commands
func (a *App) handleGlobalKey(ev *tcell.EventKey) bool {
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
		a.promptOpen()
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

func (a *App) getEditor() *Editor {
	if len(a.tabs) == 0 {
		return nil
	}
	return a.tabs[a.activeTab].editor
}

func (a *App) showPalette(prefix string) {
	p := NewPalette()
	p.input.OnChange = func() {
		text := p.input.String()
		p.list.Clear()
		p.list.Index = 0

		switch {
		case strings.HasPrefix(text, ":"):
			// 1. Go to Line
			lineStr := text[1:]
			n, err := strconv.Atoi(lineStr)
			if err != nil || n < 1 {
				return
			}

			p.list.Append(ui.ListItem{Name: "Go to Line " + lineStr, Value: n - 1})
			p.list.OnSelect = func(item ui.ListItem) {
				lineNum := item.Value.(int)
				if e := a.getEditor(); e != nil {
					e.gotoLine(lineNum)
				}
				a.requestFocus()
			}

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
			p.list.OnSelect = func(item ui.ListItem) {
				symbolLine := item.Value.(int)
				editor.gotoLine(symbolLine)
				a.requestFocus()
			}

			for _, sym := range editor.symbols {
				ok := true
				for _, word := range words {
					if word == "" {
						continue
					}
					if !strings.Contains(strings.ToLower(sym.FullName), word) {
						ok = false
						break
					}
				}
				if ok {
					p.list.Append(ui.ListItem{Name: sym.FullName, Value: sym.Line})
				}
			}

		case strings.HasPrefix(text, ">"):
			a.fillCommandMode(p, text[1:])
		default:
			a.fillFileSearchMode(p, text)
		}
	}

	p.input.SetText(prefix)
	a.manager.Overlay(p, "top")
}

func (a *App) fillCommandMode(p *Palette, query string) {
	words := strings.Fields(query)
	commands := []struct {
		name   string
		action func()
	}{
		{"Color Theme: Breaks", func() {
			ui.Theme = ui.Breakers
			a.requestFocus()
		}},
		{"Color Theme: Mariana", func() {
			ui.Theme = ui.Mariana
			a.requestFocus()
		}},
		{"Goto Definition", func() {
			if e := a.getEditor(); e != nil {
				e.gotoDefinition()
			}
			a.requestFocus()
		}},
		{"Goto Symbol", func() { a.showPalette("@") }},
		{"New File", func() { a.newTab("untitled"); a.requestFocus() }},
		{"Quit", a.manager.Stop},
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
			p.list.Append(ui.ListItem{Name: cmd.name, Value: cmd.action})
		}
	}

	p.list.OnSelect = func(item ui.ListItem) {
		action := item.Value.(func())
		action()
	}
}

func (a *App) fillFileSearchMode(p *Palette, query string) {
	p.list.OnSelect = func(item ui.ListItem) {
		a.openFile(item.Value.(string))
		a.requestFocus()
	}

	query = strings.ToLower(query)
	filter := make(map[string]bool)
	currentDir, _ := os.Getwd()
	ignoreRules := loadGitignoreRules(filepath.Join(currentDir, ".gitignore"))

	// list opened tabs first
	for _, t := range a.tabs {
		path := t.path
		// show relative path if possible
		if filepath.IsAbs(t.path) {
			if rel, err := filepath.Rel(currentDir, t.path); err == nil && !strings.HasPrefix(rel, "..") {
				path = rel
			}
		}

		if query == "" || strings.Contains(strings.ToLower(path), query) {
			p.list.Append(ui.ListItem{Name: path, Value: path})
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
		if isGitignored(name, name, ignoreRules) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(name), query) {
			continue
		}
		p.list.Append(ui.ListItem{Name: name, Value: name})
		if p.list.Len() >= 10 {
			return
		}
	}
}

type gitignoreRule struct {
	pattern  string
	negate   bool
	hasSlash bool
	dirOnly  bool
}

func loadGitignoreRules(path string) []gitignoreRule {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(bs), "\n")
	rules := make([]gitignoreRule, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = strings.TrimSpace(line[1:])
			if line == "" {
				continue
			}
		}
		dirOnly := strings.HasSuffix(line, "/")
		if dirOnly {
			line = strings.TrimSuffix(line, "/")
			if line == "" {
				continue
			}
		}
		line = strings.TrimPrefix(line, "/")
		rules = append(rules, gitignoreRule{
			pattern:  line,
			negate:   negate,
			hasSlash: strings.Contains(line, "/"),
			dirOnly:  dirOnly,
		})
	}
	return rules
}

func isGitignored(relPath, name string, rules []gitignoreRule) bool {
	if len(rules) == 0 {
		return false
	}
	ignored := false
	for _, r := range rules {
		if r.dirOnly {
			continue
		}
		var target string
		if r.hasSlash {
			target = filepath.ToSlash(relPath)
		} else {
			target = name
		}
		matched, err := filepath.Match(r.pattern, target)
		if err != nil || !matched {
			continue
		}
		ignored = !r.negate
	}
	return ignored
}

func (a *App) openFile(name string) error {
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
func (a *App) pushHistory(path string, pos ui.Pos) {
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

func (a *App) recordJump() {
	if e := a.getEditor(); e != nil && a.tabs[a.activeTab].path != "" {
		a.pushHistory(a.tabs[a.activeTab].path, e.Pos)
	}
}

func (a *App) goBack() {
	defer a.requestFocus()
	if a.historyPos <= 0 {
		return
	}
	a.historyPos--
	a.navigateToHistory()
}

func (a *App) goForward() {
	defer a.requestFocus()
	if a.historyPos >= len(a.history)-1 {
		return
	}
	a.historyPos++
	a.navigateToHistory()
}

func (a *App) navigateToHistory() {
	if a.historyPos < 0 || a.historyPos >= len(a.history) {
		return
	}
	a.navigatingHist = true
	defer func() { a.navigatingHist = false }()

	entry := a.history[a.historyPos]
	if err := a.openFile(entry.path); err != nil {
		a.setStatus(err.Error(), 3*time.Second)
		return
	}
	if e := a.getEditor(); e != nil {
		e.SetCursor(entry.pos.Row, entry.pos.Col)
		e.CenterRow(entry.pos.Row)
	}
}

func (a *App) saveFile() {
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

func (a *App) promptSaveAs(commit func(path string)) {
	input := &ui.Input{
		OnCommit: func(text string) {
			if commit != nil {
				commit(text)
			}
			a.manager.CloseOverlay()
		},
	}

	okBtn := &ui.Button{
		Text: "OK",
		OnClick: func() {
			if commit != nil {
				commit(input.String())
			}
			a.manager.CloseOverlay()
		},
		Style: ui.Style{BG: ui.Theme.Selection},
	}

	dialog := ui.Frame(ui.Border(ui.VStack(
		ui.PadH(ui.HStack(
			ui.NewText("Save as: "),
			ui.Grow(input),
		), 1),

		ui.PadH(ui.HStack(
			ui.NewButton("Cancel", a.manager.CloseOverlay),
			ui.Spacer,
			okBtn,
		), 4),
	).Spacing(1)), 40, 0)
	a.manager.Overlay(dialog, "top")
	a.manager.SetFocus(input)
}

func (a *App) promptOpen() {
	input := &ui.Input{
		OnCommit: func(text string) {
			if text != "" {
				if err := a.openFile(text); err != nil {
					log.Print(err)
					a.setStatus(err.Error(), 5*time.Second)
				}
			}
			a.manager.CloseOverlay()
			a.requestFocus()
		},
	}

	okBtn := &ui.Button{
		Text: "OK",
		OnClick: func() {
			if text := input.String(); text != "" {
				if err := a.openFile(text); err != nil {
					log.Print(err)
					a.setStatus(err.Error(), 5*time.Second)
				}
			}
			a.manager.CloseOverlay()
			a.requestFocus()
		},
		Style: ui.Style{BG: ui.Theme.Selection},
	}

	dialog := ui.Frame(ui.Border(ui.VStack(
		ui.PadH(ui.HStack(
			ui.NewText("Open file: "),
			ui.Grow(input),
		), 1),

		ui.PadH(ui.HStack(
			ui.NewButton("Cancel", a.manager.CloseOverlay),
			ui.Spacer,
			okBtn,
		), 4),
	).Spacing(1)), 40, 0)
	a.manager.Overlay(dialog, "top")
	a.manager.SetFocus(input)
}

func (a *App) writeFile(path string, e *Editor) error {
	bs := []byte(e.String())

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
	a        *App
	path     string
	closeBtn *ui.Button
	editor   *Editor
	hovered  bool
}

func newTab(root *App, label string) *tab {
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
		e.Highlighter = highlightGo
	case ".md", ".markdown":
		e.Highlighter = highlightMarkdown
	}
	t.editor = e
	return t
}

const tabItemWidth = 18

func (t *tab) Size() (int, int) { return tabItemWidth, 1 }
func (t *tab) Layout(r ui.Rect) *ui.Node {
	bw, bh := t.closeBtn.Size()
	return &ui.Node{
		Element: t,
		Rect:    r,
		Children: []*ui.Node{
			t.closeBtn.Layout(ui.Rect{X: r.X + tabItemWidth - 3, Y: r.Y, W: bw, H: bh}),
		},
	}
}
func (t *tab) Draw(screen ui.Screen, r ui.Rect) {
	style := ui.Theme.Syntax.Comment
	if t == t.a.tabs[t.a.activeTab] {
		style.FG = ui.Theme.Foreground
		style.FontUnderline = true
	} else if t.hovered {
		style.BG = ui.Theme.Hover
	}

	t.closeBtn.Style.FG = style.FG

	labelWidth := tabItemWidth - 3 - 1 // minus button and padding
	label := filepath.Base(t.path)
	if runewidth.StringWidth(label) <= labelWidth {
		label = runewidth.FillRight(label, labelWidth)
	} else {
		label = runewidth.Truncate(label, labelWidth, "…")
	}
	label = fmt.Sprintf(" %s", label)
	ui.DrawString(screen, r.X, r.Y, r.W, label, style)
}

func (t *tab) OnMouseDown(lx, ly int) {
	// like Sublime Text, instant react on clicking tab, not waiting the mouse up
	for i, tab := range t.a.tabs {
		if tab == t {
			t.a.activeTab = i
			t.a.requestFocus()
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
	input *proxyInput
	list  *ui.List
}

func NewPalette() *Palette {
	p := &Palette{
		list: new(ui.List),
	}
	// Use proxyInput to delegate key handling to Palette
	p.input = &proxyInput{
		Input:  new(ui.Input),
		parent: p,
	}
	return p
}

func (p *Palette) SetText(text string) {
	p.input.SetText(text)
	p.input.OnFocus()
}

func (p *Palette) Size() (int, int) {
	w1, h1 := 60, 1 // input box size
	// avoid full screen list items
	_, h2 := p.list.Size()
	if h2 > 15 {
		h2 = 15
	}
	return w1 + 2, h1 + h2 + 2 // +2 for the border
}

func (p *Palette) Layout(r ui.Rect) *ui.Node {
	view := ui.Border(ui.VStack(
		p.input,
		p.list,
	))
	return &ui.Node{
		Element:  p,
		Rect:     r,
		Children: []*ui.Node{view.Layout(r)},
	}
}

func (p *Palette) Draw(ui.Screen, ui.Rect) {
	// no-op
}

func (p *Palette) HandleKey(ev *tcell.EventKey) bool {
	consumed := true
	switch ev.Key() {
	case tcell.KeyDown, tcell.KeyCtrlN:
		p.list.Next()
	case tcell.KeyUp, tcell.KeyCtrlP:
		p.list.Prev()
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
	Name     string // identifier (e.g., "saveFile")
	FullName string // fully qualified identifier, for display (e.g., "(*root).saveFile", "type Foo")
	Line     int    // 0-based line number
	Kind     string // "type", "func" (for function and method)
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

			fullName := name
			if rawRecv != "" {
				// 提取 Receiver 型別並組合成 (*root).Method 格式
				parts := strings.Fields(rawRecv)
				recvType := parts[len(parts)-1]
				fullName = fmt.Sprintf("(%s).%s", recvType, name)
			}

			symbols = append(symbols, symbol{
				Name:     name,
				FullName: fullName,
				Line:     i,
				Kind:     "func",
			})
			continue
		}

		// 2. 處理型別定義
		if matches := typeRegex.FindStringSubmatch(line); len(matches) > 1 {
			name := matches[1]
			symbols = append(symbols, symbol{
				Name:     name,
				FullName: "type " + name,
				Line:     i,
				Kind:     "type",
			})
		}
	}
	return symbols
}

type SearchBar struct {
	a           *App
	input       *proxyInput
	btnPrev     *ui.Button
	btnNext     *ui.Button
	closeBtn    *ui.Button
	matches     []ui.Pos
	activeIndex int // -1 表示尚未進行導航定位
}

func NewSearchBar(r *App) *SearchBar {
	sb := &SearchBar{a: r, activeIndex: -1}
	sb.input = &proxyInput{
		Input:  new(ui.Input),
		parent: sb,
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

	query := strings.ToLower(sb.input.String())
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
	queryLen := utf8.RuneCountInString(sb.input.String())
	editor.CenterRow(m.Row)
	editor.SetSelection(m, ui.Pos{Row: m.Row, Col: m.Col + queryLen})
}

func (sb *SearchBar) Layout(r ui.Rect) *ui.Node {
	countStr := " 0/0 "
	if len(sb.matches) > 0 {
		displayIdx := sb.activeIndex + 1
		if sb.activeIndex == -1 {
			displayIdx = 0
		}
		countStr = fmt.Sprintf(" %d/%d ", displayIdx, len(sb.matches))
	}

	view := ui.HStack(
		ui.PadH(ui.NewText("Find:"), 1),
		ui.Grow(sb.input),
		ui.PadH(ui.NewText(countStr), 1),
		sb.btnPrev,
		sb.btnNext,
		sb.closeBtn,
	)
	return &ui.Node{
		Element:  sb,
		Rect:     r,
		Children: []*ui.Node{view.Layout(r)},
	}
}

func (sb *SearchBar) Size() (int, int) {
	return 10, 1
}

func (sb *SearchBar) Draw(s ui.Screen, r ui.Rect) {}

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
		sb.a.requestFocus()
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

// proxyInput delegates focus to its parent element
type proxyInput struct {
	*ui.Input
	parent ui.Element
}

func (p *proxyInput) Layout(r ui.Rect) *ui.Node {
	return &ui.Node{Element: p, Rect: r}
}

func (p *proxyInput) FocusTarget() ui.Element {
	return p.parent
}

func (a *App) activateLeader() {
	a.leaderKeyActive = true
	if a.leaderTimer != nil {
		a.leaderTimer.Stop()
	}
	a.leaderTimer = time.AfterFunc(2*time.Second, func() {
		a.leaderKeyActive = false
		a.manager.Refresh()
	})
}

// detectLightTerminal detects if terminal has a light background via COLORFGBG.
// iTerm2 sets this as "foreground;background".
// Background 7 or 15 indicates light, 0-6 and 8 indicate dark.
func detectLightTerminal() bool {
	colorfgbg := os.Getenv("COLORFGBG")
	if colorfgbg == "" {
		return false
	}
	parts := strings.Split(colorfgbg, ";")
	if len(parts) != 2 {
		return false
	}
	bg := parts[1]
	return bg == "7" || bg == "15"
}
