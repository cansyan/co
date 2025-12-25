package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
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
		root.newTab("untitled", "")
	}

	app := ui.Default()
	app.Focus(root)

	// common
	app.BindKey("Ctrl+Q", app.Close)
	app.BindKey("Ctrl+S", root.saveFile)
	app.BindKey("Ctrl+W", func() {
		root.closeTab(root.active)
	})
	app.BindKey("Ctrl+T", func() {
		root.newTab("untitled", "")
		ui.Default().Focus(root)
	})
	app.BindKey("Ctrl+F", func() {
		root.showSearch = true
		// 重置狀態，確保切換文件或重新開啟時會重新掃描
		root.searchBar.matches = nil
		root.searchBar.activeIndex = -1
		query := root.searchBar.input.Text()
		root.searchBar.input.Select(0, len([]rune(query)))
		ui.Default().Focus(root.searchBar)
	})
	app.BindKey("Ctrl+C", root.copy)
	app.BindKey("Ctrl+V", root.paste)
	app.BindKey("Ctrl+Z", nil) // undo
	app.BindKey("Ctrl+Y", nil) // redo
	app.BindKey("Ctrl+A", root.selectAll)

	// command palette
	app.BindKey("Ctrl+P", root.showCmdPalatte)
	app.BindKey("Ctrl+G", root.showLinePalette)
	app.BindKey("Ctrl+O", root.showFilePalette)
	app.BindKey("Ctrl+R", root.showSymbolPalette)

	app.BindKey("Ctrl+N", root.complete)
	app.BindKey("Ctrl+D", root.selectWord)
	app.BindKey("Ctrl+L", root.selectLine)
	app.BindKey("Esc", func() {
		focused := ui.Default().Focused()
		if editor, ok := focused.(*ui.TextEditor); ok {
			if _, _, _, _, ok := editor.Selection(); ok {
				editor.CancelSelection()
				return
			}
		}

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

// root implements ui.Element
type root struct {
	tabs       []*tab
	active     int
	btnNew     *ui.Button
	btnSave    *ui.Button
	btnQuit    *ui.Button
	status     *ui.Text
	searchBar  *SearchBar
	showSearch bool
	copyStr    string
}

func newRoot() *root {
	r := &root{
		status: ui.NewText("Ready"),
	}
	r.btnNew = ui.NewButton("New", func() {
		r.newTab("untitled", "")
		ui.Default().Focus(r)
	})
	r.btnSave = ui.NewButton("Save", r.saveFile)
	r.btnQuit = ui.NewButton("Quit", ui.Default().Close)
	r.searchBar = NewSearchBar(r)
	return r
}

func (r *root) newTab(label string, content string) {
	editor := ui.NewTextEditor()
	editor.SetText(content)
	editor.OnChange(func() {
		r.status.Label = editor.Debug()
	})
	r.tabs = append(r.tabs, newTab(r, label, editor))
	r.active = len(r.tabs) - 1
}

// closeTab closes the tab at index i, prompting to save if there are unsaved changes.
func (r *root) closeTab(i int) {
	if i < 0 || i >= len(r.tabs) {
		return
	}
	tab := r.tabs[i]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok || !editor.Dirty {
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
					err := os.WriteFile(path, []byte(editor.String()), 0644)
					if err != nil {
						log.Print(err)
						r.status.Label = err.Error()
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
					err := os.WriteFile(path, []byte(editor.String()), 0644)
					if err != nil {
						log.Print(err)
						r.status.Label = err.Error()
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
	mainStack.Append(ui.Divider(), r.status)

	n := ui.NewLayoutNode(r, x, y, w, h)
	n.Children = []*ui.LayoutNode{mainStack.Layout(x, y, w, h)}
	return n
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

func (r *root) OnFocus()                     {}
func (r *root) OnBlur()                      {}
func (r *root) HandleKey(ev *tcell.EventKey) {}

func (r *root) showCmdPalatte() {
	palette := NewPalette()
	palette.Add("Color theme: Breaks", func() {
		ui.Theme = ui.NewBreakersTheme()
	})
	palette.Add("Color theme: Mariana", func() {
		ui.Theme = ui.NewMarianaTheme()
	})
	palette.Add("New File", func() {
		r.newTab("untitled", "")
		ui.Default().Focus(r)
	})
	palette.Add("Quit", ui.Default().Close)
	ui.Default().Overlay(palette, "top")
}

func (r *root) showLinePalette() {
	p := NewPalette()
	p.SetText(":")
	p.OnSubmit = func(text string) {
		if !strings.HasPrefix(text, ":") {
			return
		}
		lineStr := strings.TrimPrefix(text, ":")
		var line int
		fmt.Sscanf(lineStr, "%d", &line)

		if tab := r.tabs[r.active]; tab != nil {
			if editor, ok := tab.body.(*ui.TextEditor); ok {
				editor.SetCursor(line-1, 0)
				editor.CenterRow(line - 1)
			}
		}
	}
	ui.Default().Overlay(p, "top")
}

func (r *root) showFilePalette() {
	p := NewPalette()

	entries, err := os.ReadDir(".")
	if err != nil {
		log.Print(err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		p.Add(name, func() {
			r.openFile(name)
			ui.Default().Focus(r)
		})
	}

	ui.Default().Overlay(p, "top")
}

func (r *root) showSymbolPalette() {
	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}

	p := NewPalette()
	// p.SetText("@")

	symbols := r.extractSymbols(tab.label, editor.String())
	if len(symbols) == 0 {
		r.status.Label = "No symbols found"
		return
	}

	for _, s := range symbols {
		p.Add(s.name, func() {
			editor.SetCursor(s.line, 0)
			editor.CenterRow(s.line)
		})
	}

	ui.Default().Overlay(p, "top")
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

	r.newTab(filepath.Base(name), string(bs))
	return nil
}

func (r *root) saveFile() {
	if len(r.tabs) == 0 {
		return
	}
	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}
	if !editor.Dirty {
		ui.Default().Focus(r)
		return
	}

	if path := tab.label; path != "untitled" {
		err := os.WriteFile(path, []byte(editor.String()), 0644)
		if err != nil {
			log.Print(err)
			r.status.Label = err.Error()
			return
		}
		editor.Dirty = false
		ui.Default().Focus(r)
		return
	}

	sa := NewSaveAs(func(path string) {
		if path == "" {
			return
		}
		err := os.WriteFile(path, []byte(editor.String()), 0644)
		if err != nil {
			log.Print(err)
			r.status.Label = err.Error()
			return
		}
		tab.label = path
		editor.Dirty = false
		ui.Default().Focus(r)
	})
	ui.Default().Overlay(sa, "center")
}

type tab struct {
	root     *root
	label    string
	btnClose *ui.Button
	body     ui.Element
	hovered  bool
	style    ui.Style
}

func newTab(root *root, label string, body ui.Element) *tab {
	t := &tab{
		root:  root,
		label: label,
		body:  body,
	}
	t.btnClose = ui.NewButton("x", func() {
		for i, tab := range root.tabs {
			if tab == t {
				t.root.closeTab(i)
				return
			}
		}
	})
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
	ui.Style
	cmds []*struct {
		Name   string
		Action func()
	}
	input    *ui.TextInput
	list     *ui.ListView
	OnSubmit func(string) // directly handle Enter
}

func NewPalette() *Palette {
	p := &Palette{
		input: ui.NewTextInput(),
		list:  ui.NewListView(),
	}
	p.list.Hovered = 0
	p.input.OnChange(func() {
		keyword := strings.ToLower(p.input.Text())
		words := []string{keyword}
		if strings.Contains(keyword, ".") {
			words = strings.Split(keyword, ".")
		} else if strings.Contains(keyword, " ") {
			words = strings.Split(keyword, " ")
		}

		p.list.Clear()
		p.list.Hovered = 0
		for _, cmd := range p.cmds {
			ok := true
			for _, word := range words {
				if word == "" {
					continue
				}
				if !strings.Contains(strings.ToLower(cmd.Name), word) {
					ok = false
					break
				}
			}
			if ok {
				p.list.Append(cmd.Name, func() {
					cmd.Action()
					ui.Default().CloseOverlay()
				})
			}
		}
	})
	return p
}

func (p *Palette) SetText(text string) {
	p.input.SetText(text)
	p.input.OnFocus()
}

func (p *Palette) Add(name string, action func()) {
	p.cmds = append(p.cmds, &struct {
		Name   string
		Action func()
	}{Name: name, Action: action})
	f := func() {
		action()
		ui.Default().CloseOverlay()
	}
	p.list.Append(name, f)
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

func (p *Palette) HandleKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyESC:
		ui.Default().CloseOverlay()
	case tcell.KeyDown:
		p.list.Hovered = (p.list.Hovered + 1) % len(p.list.Items)
	case tcell.KeyUp:
		n := len(p.list.Items)
		p.list.Hovered = (p.list.Hovered - 1 + n) % n
	case tcell.KeyEnter:
		if len(p.list.Items) > 0 {
			item := p.list.Items[p.list.Hovered]
			if item.Action != nil {
				item.Action()
			}
		} else if p.OnSubmit != nil {
			p.OnSubmit(p.input.Text())
			ui.Default().CloseOverlay()
		}
	default:
		p.input.HandleKey(ev)
	}
}

func (p *Palette) FocusTarget() ui.Element {
	return p
}
func (p *Palette) OnFocus() { p.input.OnFocus() }
func (p *Palette) OnBlur()  {}

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
			action(input.Text())
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

func (m *SaveAs) HandleKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyESC:
		ui.Default().CloseOverlay()
	case tcell.KeyEnter:
		m.btnOK.OnClick()
		ui.Default().CloseOverlay()
	default:
		m.input.HandleKey(ev)
	}
}

func (m *SaveAs) FocusTarget() ui.Element {
	return m
}

func (m *SaveAs) OnFocus() { m.input.OnFocus() }
func (m *SaveAs) OnBlur()  {}

type symbol struct {
	name string
	line int
}

func (r *root) extractSymbols(filename, content string) []symbol {
	if path.Ext(filename) != ".go" || len(content) == 0 {
		return nil
	}
	var symbols []symbol
	fset := token.NewFileSet()

	// 解析原始碼，這裡我們只需要解析宣告部分
	f, err := parser.ParseFile(fset, filename, content, 0)
	if err != nil {
		log.Printf("parser: %v", err)
		return nil
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// 處理 Function 與 Method
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				// 取得 Receiver 名稱，例如 (r *root)
				recv := ""
				typeExpr := d.Recv.List[0].Type
				switch t := typeExpr.(type) {
				case *ast.Ident:
					recv = t.Name
				case *ast.StarExpr:
					if id, ok := t.X.(*ast.Ident); ok {
						recv = "*" + id.Name
					}
				}
				name = fmt.Sprintf("(%s).%s", recv, name)
			}

			symbols = append(symbols, symbol{
				name: name,
				line: fset.Position(d.Pos()).Line - 1,
			})

		case *ast.GenDecl:
			// 處理 type 宣告 (struct, interface 等)
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						symbols = append(symbols, symbol{
							name: "type " + ts.Name.Name,
							line: fset.Position(ts.Pos()).Line - 1,
						})
					}
				}
			}
		}
	}
	return symbols
}

type match struct {
	line int
	col  int
}

type SearchBar struct {
	root        *root
	input       *proxyInput
	btnPrev     *ui.Button
	btnNext     *ui.Button
	matches     []match
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

	query := strings.ToLower(sb.input.Text())
	if query == "" {
		return
	}

	tab := sb.root.tabs[sb.root.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
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

		sb.matches = append(sb.matches, match{
			line: lineCount,
			col:  runeCol,
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
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}

	curLine, curCol := editor.Cursor()

	// 尋找第一個在游標位置之後的匹配項
	for i, m := range sb.matches {
		if m.line > curLine || (m.line == curLine && m.col >= curCol) {
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
	tab := sb.root.tabs[sb.root.active]
	if editor, ok := tab.body.(*ui.TextEditor); ok {
		queryLen := utf8.RuneCountInString(sb.input.Text())
		editor.CenterRow(m.line)
		editor.Select(m.line, m.col, m.line, m.col+queryLen)
	}
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

func (sb *SearchBar) HandleKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEnter:
		sb.navigate(true)
		// can not detect key shift+enter,
		// tcell's current implementation does not report modifier key Shift
	case tcell.KeyUp:
		sb.navigate(false)
	case tcell.KeyDown:
		sb.navigate(true)
	case tcell.KeyESC:
		// leave this as a backup, but the global
		// binding will likely catch ESC first
		sb.root.showSearch = false
		ui.Default().Focus(sb.root)
	default:
		sb.input.HandleKey(ev)
	}
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

// if no selection, select the word under the cursor;
// if has selection, jump to the next same word, like * in Vim.
//
// To keep things simple, this is not multiple selection (multi-cursor).
func (r *root) selectWord() {
	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}
	r1, c1, r2, c2, ok := editor.Selection()
	if !ok {
		editor.SelectWord()
	} else if r1 == r2 {
		query := string(editor.Line(r1)[c1:c2])
		editor.FindNext(query)
	}
}

func (r *root) selectLine() {
	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}
	editor.SelectLine()
}

func (r *root) selectAll() {
	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}

	line := editor.Line(editor.Len() - 1)
	editor.Select(0, 0, editor.Len()-1, len(line))
}

func (r *root) complete() {
	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}

	row, col := editor.Cursor()
	line := editor.Line(row)

	// word boundary
	isWordChar := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	start := col
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	end := col
	if end < len(line) && isWordChar(line[end]) {
		for end < len(line) && isWordChar(line[end]) {
			end++
		}
	}
	if start == end {
		return
	}

	word := strings.ToLower(string(line[start:end]))
	symbols := r.extractSymbols(tab.label, editor.String())
	for _, symbol := range symbols {
		if strings.HasPrefix(strings.ToLower(symbol.name), word) {
			editor.DeleteRange(row, start, row, end)
			editor.InsertText(symbol.name)
			break
		}
	}
}

// copy stores copied text to app's buffer, not OS pasteboard
func (r *root) copy() {
	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}

	s := editor.SelectedText()
	if s == "" {
		// copy current line by default
		row, _ := editor.Cursor()
		r.copyStr = string(editor.Line(row))
		return
	}

	r.copyStr = s
}

func (r *root) paste() {
	if r.copyStr == "" {
		return
	}

	tab := r.tabs[r.active]
	editor, ok := tab.body.(*ui.TextEditor)
	if !ok {
		return
	}

	editor.InsertText(r.copyStr)
}
