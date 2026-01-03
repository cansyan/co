package main

import (
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"tui/ui"
	"unicode"
	"unicode/utf8"

	"slices"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

var light = flag.Bool("light", false, "use light color theme")

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

	root := newRoot()
	if path := flag.Arg(0); path != "" {
		err := root.openFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	} else {
		root.newTab("untitled")
	}

	app := ui.Default()
	app.Focus(root)
	app.BindKey("Ctrl+Q", app.Close)
	app.BindKey("Ctrl+S", root.saveFile)
	app.BindKey("Ctrl+W", func() {
		root.closeTab(root.active)
	})
	app.BindKey("Ctrl+T", func() {
		root.newTab("untitled")
		ui.Default().Focus(root)
	})
	app.BindKey("Ctrl+F", func() {
		root.showSearch = true
		// 重置狀態，確保切換文件或重新開啟時會重新掃描
		root.searchBar.matches = nil
		root.searchBar.activeIndex = -1
		query := root.searchBar.input.String()
		root.searchBar.input.Select(0, len([]rune(query)))
		ui.Default().Focus(root.searchBar)
	})

	// command palette
	app.BindKey("Ctrl+K", func() {
		root.activateLeader()
	})
	app.BindKey("Ctrl+P", func() {
		if root.leaderKeyActive {
			root.leaderKeyActive = false
			if root.leaderTimer != nil {
				root.leaderTimer.Stop()
			}
			root.showPalette(">") // Ctrl+K Ctrl+P 進入指令模式
		} else {
			root.showPalette("") // 預設模式：檔案搜尋
		}
	})
	app.BindKey("Ctrl+R", func() { root.showPalette("@") }) // go to symbol
	app.BindKey("Esc", func() {
		if root.showSearch {
			root.showSearch = false
			ui.Default().Focus(root)
			return
		}

		app.CloseOverlay()
	})

	if err := app.Serve(root); err != nil {
		log.Print(err)
		return
	}
}

var _ ui.Focusable = (*root)(nil)

// root implements ui.Element
type root struct {
	tabs    []*tab
	active  int
	btnNew  *ui.Button
	btnSave *ui.Button
	btnQuit *ui.Button
	status  string

	searchBar  *SearchBar
	showSearch bool
	copyStr    string

	leaderKeyActive bool
	leaderTimer     *time.Timer
}

func newRoot() *root {
	r := &root{}
	r.btnNew = ui.NewButton("New", func() {
		r.newTab("untitled")
		ui.Default().Focus(r)
	}).NoFeedback()
	r.btnSave = ui.NewButton("Save", r.saveFile).NoFeedback()
	r.btnQuit = ui.NewButton("Quit", ui.Default().Close).NoFeedback()
	r.searchBar = NewSearchBar(r)
	return r
}

func (r *root) newTab(label string) {
	r.tabs = append(r.tabs, newTab(r, label))
	r.active = len(r.tabs) - 1
}

// closeTab closes the tab at index i, prompting to save if there are unsaved changes.
func (r *root) closeTab(i int) {
	if i < 0 || i >= len(r.tabs) {
		return
	}
	tab := r.tabs[i]
	editor := tab.body
	if !editor.Dirty {
		r.deleteTab(i)
		ui.Default().Focus(r)
		return
	}

	// Prompt to save changes.
	view := ui.Border(ui.VStack(
		ui.PadH(ui.NewText("Save the changes before closing?"), 1),
		ui.PadH(ui.HStack(
			ui.NewButton("Don't Save", func() {
				r.deleteTab(i)
				ui.Default().CloseOverlay()
				ui.Default().Focus(r)
			}),

			ui.PadH(ui.NewButton("Cancel", func() {
				ui.Default().CloseOverlay()
			}), 2),

			ui.NewButton("Save", func() {
				if path := tab.label; path != "untitled" {
					if err := r.writeFile(path, editor); err != nil {
						log.Print(err)
						r.setStatus(err.Error(), 5*time.Second)
						return
					}
					r.deleteTab(i)
					ui.Default().CloseOverlay()
					return
				}

				sa := NewSaveAs(func(path string) {
					if path == "" {
						return
					}
					if err := r.writeFile(path, editor); err != nil {
						log.Print(err)
						r.setStatus(err.Error(), 5*time.Second)
						return
					}
					r.deleteTab(i)
				})
				ui.Default().Overlay(sa, "center")
			}).Background(ui.Theme.Selection),
		), 2),
	).Spacing(1))
	ui.Default().Overlay(view, "top")
}

func (r *root) deleteTab(i int) {
	if i < 0 || i >= len(r.tabs) {
		return
	}

	r.tabs = slices.Delete(r.tabs, i, i+1)
	if i < r.active {
		r.active--
	} else if i == r.active {
		r.active = max(0, len(r.tabs)-1)
	}

	if len(r.tabs) == 0 {
		ui.Default().Close()
	}
}

func (r *root) MinSize() (int, int) {
	var maxW, maxH int
	for _, t := range r.tabs {
		w, h := t.body.MinSize()
		if w > maxW {
			maxW = w
		}
		if h > maxH {
			maxH = h
		}
	}
	return maxW, maxH + 1 // +1 for tab label
}

func (r *root) Layout(x, y, w, h int) *ui.LayoutNode {
	tabLabels := ui.HStack()
	for i, tab := range r.tabs {
		tabLabels.Append(tab)
		if i != len(r.tabs)-1 {
			tabLabels.Append(ui.Divider())
		}
	}

	mainStack := ui.VStack()
	mainStack.Append(
		ui.HStack(ui.Grow(tabLabels), r.btnNew, r.btnSave, r.btnQuit),
	)
	if len(r.tabs) > 0 {
		mainStack.Append(ui.Grow(r.tabs[r.active].body))
	}
	if r.showSearch {
		mainStack.Append(ui.Divider(), r.searchBar)
	}

	statusBar := ui.HStack()
	if e := r.getEditor(); e != nil {
		posInfo := fmt.Sprintf("Line %d, Column %d", e.Pos.Row+1, e.Pos.Col+1)
		statusBar.Append(ui.NewText(posInfo))
	}
	if r.status != "" {
		statusBar.Append(ui.Spacer, ui.NewText(r.status))
	}
	if r.leaderKeyActive {
		statusBar.Append(ui.Spacer, ui.NewText("Wait for key..."))
	}

	mainStack.Append(
		ui.Divider(),
		ui.PadH(statusBar, 1),
	)

	n := ui.NewLayoutNode(r, x, y, w, h)
	n.Children = []*ui.LayoutNode{mainStack.Layout(x, y, w, h)}
	return n
}

// setStatus sets the status text and clears it after a delay.
func (r *root) setStatus(msg string, delay time.Duration) {
	r.status = msg
	time.AfterFunc(delay, func() {
		// Only clear if the current message is still the one we set.
		// (Avoid clearing a newer, different message).
		if r.status == msg {
			r.status = ""
			ui.Default().Post()
		}
	})
}

func (r *root) Render(ui.Screen, ui.Rect) {
	// no-op
}

func (r *root) FocusTarget() ui.Element {
	if len(r.tabs) == 0 {
		return r
	}
	return r.tabs[r.active].body
}

func (r *root) OnFocus()                          {}
func (r *root) OnBlur()                           {}
func (r *root) HandleKey(ev *tcell.EventKey) bool { return false }
func (r *root) getEditor() *Editor {
	if len(r.tabs) == 0 {
		return nil
	}
	return r.tabs[r.active].body
}
func (r *root) showPalette(prefix string) {
	p := NewPalette()
	p.input.OnChange(func() {
		text := p.input.String()
		p.list.Clear()
		p.list.Hovered = 0

		switch {
		case strings.HasPrefix(text, ":"):
			editor := r.getEditor()
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
				ui.Default().Focus(r)
			})

		case strings.HasPrefix(text, "@"):
			// 2. Go to Symbol
			query := strings.ToLower(text[1:])
			// Split by spaces or dots
			words := strings.FieldsFunc(query, func(r rune) bool {
				return r == ' ' || r == '.'
			})
			editor := r.getEditor()
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
						ui.Default().Focus(r)
					})
				}
			}

		case strings.HasPrefix(text, ">"):
			// 3. Command Mode
			r.fillCommandMode(p, text[1:])
		default:
			// 4. File Search Mode (Default)
			query := strings.ToLower(text)
			entries, _ := os.ReadDir(".")
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if query == "" || strings.Contains(strings.ToLower(name), query) {
					p.list.Append(name, func() {
						r.openFile(name)
						ui.Default().Focus(r)
					})
				}
				// 10 results is fine
				if len(p.list.Items) >= 10 {
					return
				}
			}
		}
	})

	p.input.SetText(prefix)
	ui.Default().Overlay(p, "top")
}

func (r *root) fillCommandMode(p *Palette, query string) {
	words := strings.Fields(query)
	commands := []struct {
		name   string
		action func()
	}{
		{"Color Theme: Breaks", func() { ui.Theme = ui.NewBreakersTheme() }},
		{"Color Theme: Mariana", func() { ui.Theme = ui.NewMarianaTheme() }},
		{"Goto Definition", func() {
			if e := r.getEditor(); e != nil {
				e.gotoDefinition()
			}
		}},
		{"Goto Symbol", func() { r.showPalette("@") }},
		{"New File", func() { r.newTab("untitled"); ui.Default().Focus(r) }},
		{"Quit", ui.Default().Close},
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
				ui.Default().Focus(r)
			})
		}
	}
}

func (r *root) openFile(name string) error {
	// tab existed, just switch
	for i, tab := range r.tabs {
		if tab.label == filepath.Base(name) {
			r.active = i
			return nil
		}
	}

	bs, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	r.newTab(filepath.Base(name))
	r.getEditor().SetText(string(bs))
	return nil
}

func (r *root) saveFile() {
	if len(r.tabs) == 0 {
		return
	}
	tab := r.tabs[r.active]
	editor := tab.body
	if !editor.Dirty {
		ui.Default().Focus(r)
		return
	}

	if path := tab.label; path != "untitled" {
		if err := r.writeFile(path, editor); err != nil {
			log.Print(err)
			r.setStatus(err.Error(), 5*time.Second)
			return
		}
		r.setStatus("Saved "+path, 2*time.Second)
		ui.Default().Focus(r)
		return
	}

	sa := NewSaveAs(func(path string) {
		if path == "" {
			return
		}
		if err := r.writeFile(path, editor); err != nil {
			log.Print(err)
			r.setStatus(err.Error(), 5*time.Second)
			return
		}
		tab.label = path
		ui.Default().Focus(r)
	})
	ui.Default().Overlay(sa, "center")
}

func (r *root) writeFile(path string, e *Editor) error {
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
			r.setStatus(fmt.Sprintf("Format error: %v", err), 5*time.Second)
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
	root     *root
	label    string
	btnClose *ui.Button
	body     *Editor
	hovered  bool
	style    ui.Style
}

func newTab(root *root, label string) *tab {
	t := &tab{
		root:  root,
		label: label,
	}
	t.btnClose = ui.NewButton("x", func() {
		for i, tab := range root.tabs {
			if tab == t {
				t.root.closeTab(i)
				return
			}
		}
	})
	e := NewEditor(root)
	if filepath.Ext(label) == ".go" {
		e.SetLanguage("go")
	}
	t.body = e
	return t
}

const tabItemWidth = 15

func (t *tab) MinSize() (int, int) { return tabItemWidth, 1 }
func (t *tab) Layout(x, y, w, h int) *ui.LayoutNode {
	bw, bh := t.btnClose.MinSize()
	return &ui.LayoutNode{
		Element: t,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
		Children: []*ui.LayoutNode{
			t.btnClose.Layout(x+tabItemWidth-3, y, bw, bh),
		},
	}
}
func (t *tab) Render(screen ui.Screen, r ui.Rect) {
	var st ui.Style
	if t == t.root.tabs[t.root.active] {
		st.FontUnderline = true
		st = t.style.Merge(st)
	} else if t.hovered {
		st.BG = ui.Theme.Hover
		st = t.style.Merge(st)
	}

	format := " %s"
	labelWidth := tabItemWidth - 3 - 1 // minus button and padding
	var label string
	if runewidth.StringWidth(t.label) <= labelWidth {
		label = runewidth.FillRight(t.label, labelWidth)
	} else {
		label = runewidth.Truncate(t.label, labelWidth, "…")
	}
	label = fmt.Sprintf(format, label)
	ui.DrawString(screen, r.X, r.Y, r.W, label, st.Apply())
}

func (t *tab) OnMouseDown(lx, ly int) {
	// like Sublime Text, instant react on clicking tab, not waiting the mouse up
	for i, tab := range t.root.tabs {
		if tab == t {
			t.root.active = i
			ui.Default().Focus(t.root)
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
		input: ui.NewTextInput(),
		list:  ui.NewListView(),
	}
	p.list.Hovered = 0
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
	case tcell.KeyESC:
		ui.Default().CloseOverlay()
	case tcell.KeyDown, tcell.KeyCtrlN:
		p.list.Hovered = (p.list.Hovered + 1) % len(p.list.Items)
	case tcell.KeyUp, tcell.KeyCtrlP:
		n := len(p.list.Items)
		p.list.Hovered = (p.list.Hovered - 1 + n) % n
	case tcell.KeyEnter:
		if len(p.list.Items) > 0 {
			item := p.list.Items[p.list.Hovered]
			if item.Action != nil {
				item.Action()
			}
		}
	default:
		p.input.HandleKey(ev)
		consumed = false
	}
	return consumed
}

func (p *Palette) FocusTarget() ui.Element { return p }
func (p *Palette) OnFocus()                { p.input.OnFocus() }
func (p *Palette) OnBlur()                 { p.input.OnBlur() }

type SaveAs struct {
	child ui.Element
	btnOK *ui.Button
	input *ui.TextInput
}

func NewSaveAs(action func(string)) *SaveAs {
	msg := ui.NewText("Save as: ")
	input := new(ui.TextInput)
	btnCancel := ui.NewButton("Cancel", func() {
		ui.Default().CloseOverlay()
	})
	btnOK := ui.NewButton("OK", func() {
		if action != nil {
			action(input.String())
		}
		ui.Default().CloseOverlay()
	}).Background(ui.Theme.Selection)

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
		28, 0,
	)

	return &SaveAs{
		child: view,
		btnOK: btnOK,
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

func (m *SaveAs) HandleKey(ev *tcell.EventKey) bool {
	consumed := true
	switch ev.Key() {
	case tcell.KeyESC:
		ui.Default().CloseOverlay()
	case tcell.KeyEnter:
		m.btnOK.OnClick()
		ui.Default().CloseOverlay()
	default:
		m.input.HandleKey(ev)
		consumed = false
	}
	return consumed
}

func (m *SaveAs) FocusTarget() ui.Element {
	return m
}

func (m *SaveAs) OnFocus() { m.input.OnFocus() }
func (m *SaveAs) OnBlur()  {}

type symbol struct {
	Name      string // 原始識別碼 (e.g., "saveFile"), 用於程式碼補全
	Signature string // 完整定義 (e.g., "(*root).saveFile"), 用於 Palette 顯示
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
	root        *root
	input       *proxyInput
	btnPrev     *ui.Button
	btnNext     *ui.Button
	matches     []ui.Pos
	activeIndex int // -1 表示尚未進行導航定位
}

func NewSearchBar(r *root) *SearchBar {
	sb := &SearchBar{root: r, activeIndex: -1}
	sb.input = &proxyInput{
		TextInput: ui.NewTextInput(),
		parent:    sb,
	}
	// Lazy Evaluation:
	// 當文字改變時，僅標記狀態為「需要重新掃描」，但不立即掃描
	// 真正的計算成本被推遲到了使用者按下 Enter、Next 或 Prev 的那一刻
	sb.input.OnChange(func() {
		sb.matches = nil
		sb.activeIndex = -1
	})

	sb.btnPrev = ui.NewButton("< Prev", func() { sb.navigate(false) })
	sb.btnNext = ui.NewButton("Next >", func() { sb.navigate(true) })
	return sb
}

func (sb *SearchBar) updateMatches() {
	sb.matches = nil
	sb.activeIndex = -1

	query := strings.ToLower(sb.input.String())
	if query == "" {
		return
	}

	editor := sb.root.getEditor()
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

	tab := sb.root.tabs[sb.root.active]
	e := tab.body

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
	editor := sb.root.getEditor()
	if editor == nil {
		return
	}
	queryLen := utf8.RuneCountInString(sb.input.String())
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
		// leave this as a backup, but the global
		// binding will likely catch ESC first
		sb.root.showSearch = false
		ui.Default().Focus(sb.root)
	default:
		sb.input.HandleKey(ev)
		consumed = false
	}
	return consumed
}

func (sb *SearchBar) FocusTarget() ui.Element { return sb }
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

func (r *root) activateLeader() {
	r.leaderKeyActive = true
	if r.leaderTimer != nil {
		r.leaderTimer.Stop()
	}
	// 2 秒後自動重置狀態
	r.leaderTimer = time.AfterFunc(2*time.Second, func() {
		r.leaderKeyActive = false
		ui.Default().Post()
	})
}

type Editor struct {
	*ui.TextEditor
	r *root
}

func NewEditor(r *root) *Editor {
	return &Editor{
		TextEditor: ui.NewTextEditor(),
		r:          r,
	}
}

func (e *Editor) Layout(x, y, w, h int) *ui.LayoutNode {
	return &ui.LayoutNode{
		Element: e,
		Rect:    ui.Rect{X: x, Y: y, W: w, H: h},
	}
}

func (e *Editor) FocusTarget() ui.Element {
	return e
}

func (e *Editor) HandleKey(ev *tcell.EventKey) bool {
	switch strings.ToLower(ev.Name()) {
	case "ctrl+a":
		lastLine := e.Line(e.Len() - 1)
		e.SetSelection(ui.Pos{}, ui.Pos{Row: e.Len() - 1, Col: len(lastLine)})
	case "ctrl+c":
		s := e.SelectedText()
		if s == "" {
			// copy current line by default
			e.r.copyStr = string(e.Line(e.Pos.Row))
			return true
		}
		e.r.copyStr = s
	case "ctrl+x":
		start, end, ok := e.Selection()
		if !ok {
			// cut line by default
			e.r.copyStr = string(e.Line(e.Pos.Row)) + "\n"
			e.DeleteRange(ui.Pos{Row: e.Pos.Row}, ui.Pos{Row: e.Pos.Row + 1})
			return true
		}

		e.r.copyStr = e.SelectedText()
		e.DeleteRange(start, end)
	case "ctrl+v":
		if e.r.copyStr == "" {
			return true
		}
		e.InsertText(e.r.copyStr)
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
				return true
			}
		}
	case "alt+right": // goto the end of line
		e.ClearSelection()
		e.SetCursor(e.Pos.Row, len(e.Line(e.Pos.Row)))
	case "esc":
		if _, _, ok := e.Selection(); ok {
			e.ClearSelection()
			return true
		}
		return false
	default:
		return e.TextEditor.HandleKey(ev)
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
