package main

import (
	"flag"
	"fmt"
	"go/format"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"tui/ui"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

var light = flag.Bool("light", false, "use light color theme")
var app *ui.App

func main() {
	flag.Parse()
	if *light {
		ui.Theme = ui.NewBreakersTheme()
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	f, err := os.OpenFile("/tmp/tui.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.SetOutput(f)
	ui.Logger = log.Default()

	app = ui.NewApp()
	app.BindKey("Ctrl+Q", app.Stop)

	editorApp := newEditorApp()
	if path := flag.Arg(0); path != "" {
		err := editorApp.openFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	} else {
		editorApp.newTab("untitled")
	}
	app.SetFocus(editorApp)

	if err := app.Run(editorApp); err != nil {
		log.Print(err)
		return
	}
}

var _ ui.Focusable = (*EditorApp)(nil)

type EditorApp struct {
	tabs      []*tab
	activeTab int
	newBtn    *ui.Button
	saveBtn   *ui.Button
	quitBtn   *ui.Button
	status    string

	searchBar  *SearchBar
	showSearch bool
	clipboard  string

	leaderKeyActive bool
	leaderTimer     *time.Timer
}

func newEditorApp() *EditorApp {
	r := &EditorApp{}
	r.newBtn = &ui.Button{
		Text: "New",
		OnClick: func() {
			r.newTab("untitled")
			app.SetFocus(r)
		},
		// disable the menu button's feedback, less noise
		NoFeedback: true,
	}

	r.saveBtn = &ui.Button{Text: "Save", OnClick: r.saveFile, NoFeedback: true}
	r.quitBtn = &ui.Button{Text: "Quit", OnClick: app.Stop, NoFeedback: true}
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

	// Prompt to save changes.
	view := ui.Border(ui.VStack(
		ui.PadH(ui.NewText("Save the changes before closing?"), 1),
		ui.PadH(ui.HStack(
			ui.NewButton("Don't Save", func() {
				a.deleteTab(i)
				app.CloseOverlay()
				a.requestFocus()
			}),

			ui.PadH(ui.NewButton("Cancel", func() {
				app.CloseOverlay()
			}), 2),

			ui.NewButton("Save", func() {
				if path := tab.path; path != "untitled" {
					if err := a.writeFile(path, editor); err != nil {
						log.Print(err)
						a.setStatus(err.Error(), 5*time.Second)
						return
					}
					a.deleteTab(i)
					app.CloseOverlay()
					return
				}

				sa := NewSaveAs(func(path string) {
					if path == "" {
						return
					}
					if err := a.writeFile(path, editor); err != nil {
						log.Print(err)
						a.setStatus(err.Error(), 5*time.Second)
						return
					}
					a.deleteTab(i)
				})
				app.Overlay(sa, "top")
			}).SetBackground(ui.Theme.Selection),
		), 2),
	).Spacing(1))
	app.Overlay(view, "top")
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
		app.Stop()
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
			tabLabels.Append(ui.Divider())
		}
	}

	mainStack := ui.VStack()
	mainStack.Append(
		ui.HStack(ui.Grow(tabLabels), a.newBtn, a.saveBtn, a.quitBtn),
	)
	if len(a.tabs) > 0 {
		mainStack.Append(ui.Grow(a.tabs[a.activeTab].editor))
	}
	if a.showSearch {
		mainStack.Append(ui.Divider(), a.searchBar)
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
		ui.Divider(),
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
			app.Refresh()
		}
	})
}

func (a *EditorApp) Render(ui.Screen, ui.Rect) {
	// no-op
}

// a convenient method to request focus back to the app.
func (a *EditorApp) requestFocus() {
	app.SetFocus(a)
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

	app.SetFocus(sb)
}

func (a *EditorApp) openFileDialog() {
	input := new(ui.TextInput)
	input.SetPlaceholder("Open file path: ")
	input.OnCommit(func() {
		if text := input.Text(); text != "" {
			if err := a.openFile(text); err != nil {
				log.Print(err)
				a.setStatus(err.Error(), 3*time.Second)
			}
		}
		app.CloseOverlay()
		a.requestFocus()
	})
	view := ui.Border(ui.Frame(input, 60, 1))
	app.Overlay(view, "top")
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
	p.input.OnChange(func() {
		text := p.input.Text()
		p.list.Clear()
		p.list.Selected = 0

		switch {
		case strings.HasPrefix(text, ":"):
			editor := a.getEditor()
			if editor == nil {
				return
			}

			// 1. Go to Line
			lineStr := text[1:]
			line := 1
			if _, err := fmt.Sscanf(lineStr, "%d", &line); err != nil || line < 1 {
				p.list.Append(fmt.Sprintf("type line number between 1 and %d", editor.Len()), nil)
				return
			}

			p.list.Append(fmt.Sprintf("Go to Line %d", line), func() {
				editor.SetCursor(line-1, 0)
				editor.CenterRow(line - 1)
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

			symbols := extractSymbols(editor.String())
			for _, s := range symbols {
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
						editor.SetCursor(s.Line, 0)
						editor.CenterRow(s.Line)
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
	})

	p.input.SetText(prefix)
	app.Overlay(p, "top")
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
		{"Quit", app.Stop},
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
			if len(p.list.Items) >= 10 {
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
		if len(p.list.Items) >= 10 {
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
			return nil
		}
	}

	bs, err := os.ReadFile(abs)
	if err != nil {
		return err
	}

	a.newTab(abs)
	a.getEditor().SetText(string(bs))
	return nil
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

	sa := NewSaveAs(func(path string) {
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
	app.Overlay(sa, "top")
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
	return nil
}

type tab struct {
	app      *EditorApp
	path     string
	closeBtn *ui.Button
	editor   *Editor
	hovered  bool
	style    ui.Style
}

func newTab(root *EditorApp, label string) *tab {
	t := &tab{
		app:  root,
		path: label,
	}
	t.closeBtn = ui.NewButton("x", func() {
		for i, tab := range root.tabs {
			if tab == t {
				t.app.closeTab(i)
				return
			}
		}
	})
	e := NewEditor(root)
	if filepath.Ext(label) == ".go" {
		e.SetHighlighter(highlightGo)
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
	var st ui.Style
	if t == t.app.tabs[t.app.activeTab] {
		st.FontUnderline = true
		st = t.style.Merge(st)
	} else if t.hovered {
		st.BG = ui.Theme.Hover
		st = t.style.Merge(st)
	}

	labelWidth := tabItemWidth - 3 - 1 // minus button and padding
	label := filepath.Base(t.path)
	if runewidth.StringWidth(label) <= labelWidth {
		label = runewidth.FillRight(label, labelWidth)
	} else {
		label = runewidth.Truncate(label, labelWidth, "…")
	}
	label = fmt.Sprintf(" %s", label)
	ui.DrawString(screen, r.X, r.Y, r.W, label, st.Apply())
}

func (t *tab) OnMouseDown(lx, ly int) {
	// like Sublime Text, instant react on clicking tab, not waiting the mouse up
	for i, tab := range t.app.tabs {
		if tab == t {
			t.app.activeTab = i
			app.SetFocus(t.app)
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

type SaveAs struct {
	child ui.Element
	input *ui.TextInput
}

func NewSaveAs(action func(string)) *SaveAs {
	msg := ui.NewText("Save as: ")
	input := new(ui.TextInput)
	commit := func() {
		if action != nil {
			action(input.Text())
		}
		app.CloseOverlay()
	}
	input.OnCommit(commit)
	btnCancel := ui.NewButton("Cancel", func() {
		app.CloseOverlay()
	})
	btnOK := ui.NewButton("OK", commit).SetBackground(ui.Theme.Selection)

	view := ui.Frame(
		ui.Border(ui.VStack(
			ui.PadH(ui.HStack(
				msg,
				ui.Grow(input),
			), 1),

			ui.PadH(ui.HStack(
				btnCancel,
				ui.Spacer,
				btnOK,
			), 4),
		).Spacing(1)),
		40, 0,
	)

	return &SaveAs{
		child: view,
		input: input,
	}
}

func (m *SaveAs) MinSize() (int, int) {
	return m.child.MinSize()
}

func (m *SaveAs) Layout(x, y, w, h int) *ui.LayoutNode {
	node := &ui.LayoutNode{
		Element: m,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
	node.Children = append(node.Children, m.child.Layout(x, y, w, h))
	return node
}

func (m *SaveAs) Render(s ui.Screen, r ui.Rect) {}

func (m *SaveAs) FocusTarget() ui.Element {
	return m.input
}

type symbol struct {
	Name      string // identifier, for example "saveFile"
	Signature string // readable definition, for example "(*root).saveFile"
	Line      int    // line number
	Kind      string // func, type
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
	sb.input.OnChange(func() {
		sb.matches = nil
		sb.activeIndex = -1
	})

	sb.btnPrev = ui.NewButton("<", func() { sb.navigate(false) })
	sb.btnNext = ui.NewButton(">", func() { sb.navigate(true) })
	sb.closeBtn = ui.NewButton("x", func() {
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
		app.SetFocus(sb.a)
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
		app.Refresh()
	})
}

type Editor struct {
	*ui.TextEditor
	app *EditorApp
}

func NewEditor(r *EditorApp) *Editor {
	return &Editor{
		TextEditor: ui.NewTextEditor(),
		app:        r,
	}
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
		start, end, ok := e.Selection()
		if !ok {
			// cut line by default
			e.app.clipboard = string(e.Line(e.Pos.Row)) + "\n"
			e.DeleteRange(ui.Pos{Row: e.Pos.Row}, ui.Pos{Row: e.Pos.Row + 1})
			return true
		}

		e.app.clipboard = e.SelectedText()
		e.DeleteRange(start, end)
	case "ctrl+v":
		e.InsertText(e.app.clipboard)
	case "ctrl+n":
		e.autoComplete()
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
		e.SetCursor(0, 0)
		e.CenterRow(0)
	case "alt+down": // goto last line
		e.SetCursor(e.Len()-1, 0)
		e.CenterRow(e.Len() - 1)
	case "alt+left": // goto the first non-whitespace character
		e.ClearSelection()
		for i, char := range e.Line(e.Pos.Row) {
			if !unicode.IsSpace(char) {
				e.SetCursor(e.Pos.Row, i)
				break
			}
		}
	case "alt+right": // goto the end of line
		e.ClearSelection()
		e.SetCursor(e.Pos.Row, len(e.Line(e.Pos.Row)))
	case "esc":
		if _, _, ok := e.Selection(); !ok {
			return e.app.handleGlobalKey(ev)
		}
		e.ClearSelection()
	default:
		if !e.TextEditor.HandleKey(ev) {
			// Bubble event to parent
			return e.app.handleGlobalKey(ev)
		}
	}
	return true
}

func (e *Editor) gotoDefinition() {
	start, end, ok := e.WordRangeAtCursor()
	if !ok {
		return
	}
	word := string(e.Line(e.Pos.Row)[start:end])

	symbols := extractSymbols(e.String())
	for _, s := range symbols {
		if s.Name == word {
			// Move the cursor to the symbol's definition
			e.SetCursor(s.Line, 0)
			e.CenterRow(s.Line)
			return
		}
	}
}

func (e *Editor) autoComplete() {
	start, end, ok := e.WordRangeAtCursor()
	if !ok {
		return
	}
	word := string(e.Line(e.Pos.Row)[start:end])

	symbols := extractSymbols(e.String())
	for _, s := range symbols {
		if strings.HasPrefix(strings.ToLower(s.Name), word) {
			// 如果補全建議跟現在長得一模一樣，跳過，嘗試下一個
			if s.Name == word {
				continue
			}

			e.DeleteRange(ui.Pos{Row: e.Pos.Row, Col: start}, ui.Pos{Row: e.Pos.Row, Col: end})
			e.InsertText(s.Name)
			if s.Kind == "func" {
				e.InsertText("()")
				e.SetCursor(e.Pos.Row, e.Pos.Col-1)
			}
			return
		}
	}
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
