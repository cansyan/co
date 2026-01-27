package main

import (
	"testing"

	"github.com/cansyan/co/ui"
)

func TestNewEditor(t *testing.T) {
	manager := ui.NewManager()
	app := newApp(manager)
	editor := NewEditor(app)

	if editor == nil {
		t.Fatal("NewEditor returned nil")
	}

	if editor.TextEditor == nil {
		t.Fatal("Editor TextEditor is nil")
	}

	if editor.app != app {
		t.Error("Editor app not set correctly")
	}

	if !editor.InlineSuggest {
		t.Error("InlineSuggest should be enabled by default")
	}
}

func TestExtractSymbols(t *testing.T) {
	goCode := `package main

import "fmt"

func main() {
	fmt.Println("Hello")
}

type MyStruct struct {
	Field string
}

func (m *MyStruct) Method() {
	// method body
}
`

	symbols := extractSymbols(goCode)

	// Check function extraction
	var mainFunc, methodFunc *symbol
	var myStruct *symbol

	for i, sym := range symbols {
		switch sym.Name {
		case "main":
			mainFunc = &symbols[i]
		case "Method":
			methodFunc = &symbols[i]
		case "MyStruct":
			myStruct = &symbols[i]
		}
	}

	if mainFunc == nil {
		t.Error("Failed to extract main function")
	} else if mainFunc.Kind != "func" {
		t.Errorf("main function kind incorrect. Expected: 'func', Got: %q", mainFunc.Kind)
	}

	if myStruct == nil {
		t.Error("Failed to extract MyStruct type")
	} else if myStruct.Kind != "type" {
		t.Errorf("MyStruct kind incorrect. Expected: 'type', Got: %q", myStruct.Kind)
	}

	if methodFunc == nil {
		t.Error("Failed to extract Method")
	} else if methodFunc.Kind != "func" {
		t.Errorf("Method kind incorrect. Expected: 'func', Got: %q", methodFunc.Kind)
	}
}

func TestEditorGotoDefinition(t *testing.T) {
	manager := ui.NewManager()
	app := newApp(manager)
	editor := NewEditor(app)

	goCode := `package main
func main() {
	helper()
}

func helper() {
	// helper
}
`

	editor.SetText(goCode)
	editor.updateSymbols()

	// Position cursor on 'main' call and test goto definition
	editor.SetCursor(2, 1) // On 'helper()' call
	editor.gotoDefinition()

	// Should jump to main function definition
	if editor.Pos.Row != 5 {
		t.Errorf("gotoDefinition failed. Expected row 5, Got: %d", editor.Pos.Row)
	}
}

func TestSearchBarUpdateMatches(t *testing.T) {
	manager := ui.NewManager()
	app := newApp(manager)
	app.tabs = []*tab{newTab(app, "test")}
	app.activeTab = 0

	editor := NewEditor(app)
	editor.SetText("Hello world\nHello universe\nHello galaxy")
	app.tabs[0].editor = editor

	searchBar := NewSearchBar(app)
	searchBar.input.SetText("Hello")
	searchBar.updateMatches()

	if len(searchBar.matches) != 3 {
		t.Errorf("SearchBar updateMatches failed. Expected 3 matches, Got: %d", len(searchBar.matches))
	}

	// Test first match position
	firstMatch := searchBar.matches[0]
	if firstMatch.Row != 0 || firstMatch.Col != 0 {
		t.Errorf("First match incorrect. Expected: (0, 0), Got: (%d, %d)", firstMatch.Row, firstMatch.Col)
	}
}

func TestSearchBarNavigation(t *testing.T) {
	manager := ui.NewManager()
	app := newApp(manager)
	app.tabs = []*tab{newTab(app, "test")}
	app.activeTab = 0

	editor := NewEditor(app)
	editor.SetText("Hello world\nHello universe")
	app.tabs[0].editor = editor

	searchBar := NewSearchBar(app)
	searchBar.input.SetText("Hello")
	searchBar.updateMatches()

	// Test forward navigation
	searchBar.navigate(true)
	if searchBar.activeIndex != 0 {
		t.Errorf("SearchBar navigate forward failed. Expected index 0, Got: %d", searchBar.activeIndex)
	}

	// Test backward navigation - should go to previous match (index 1, not 2)
	searchBar.navigate(false)
	if searchBar.activeIndex != 1 {
		t.Errorf("SearchBar navigate backward failed. Expected index 1, Got: %d", searchBar.activeIndex)
	}
}

func TestAppTabManagement(t *testing.T) {
	manager := ui.NewManager()
	app := newApp(manager)

	// Test new tab creation
	initialCount := len(app.tabs)
	app.newTab("test.txt")
	if len(app.tabs) != initialCount+1 {
		t.Errorf("newTab failed. Expected: %d tabs, Got: %d", initialCount+1, len(app.tabs))
	}

	if app.activeTab != initialCount {
		t.Errorf("activeTab not set correctly. Expected: %d, Got: %d", initialCount, app.activeTab)
	}

	// Test tab deletion
	app.deleteTab(app.activeTab)
	if len(app.tabs) != initialCount {
		t.Errorf("deleteTab failed. Expected: %d tabs, Got: %d", initialCount, len(app.tabs))
	}
}

func TestAppHistory(t *testing.T) {
	manager := ui.NewManager()
	app := newApp(manager)

	// Test history recording
	app.pushHistory("/path/to/file", ui.Pos{Row: 5, Col: 10})
	if len(app.history) != 1 {
		t.Errorf("pushHistory failed. Expected 1 entry, Got: %d", len(app.history))
	}

	if app.historyPos != 0 {
		t.Errorf("historyPos incorrect. Expected 0, Got: %d", app.historyPos)
	}

	// Test navigation
	app.pushHistory("/path/to/another", ui.Pos{Row: 2, Col: 5})
	app.goBack()
	if app.historyPos != 0 {
		t.Errorf("goBack failed. Expected historyPos 0, Got: %d", app.historyPos)
	}

	app.goForward()
	if app.historyPos != 1 {
		t.Errorf("goForward failed. Expected historyPos 1, Got: %d", app.historyPos)
	}
}

func TestParseFileArg(t *testing.T) {
	tests := []struct {
		input string
		path  string
		line  int
	}{
		{"file.txt", "file.txt", 0},
		{"file.txt:10", "file.txt", 10},
		{"/path/to/file.txt:25", "/path/to/file.txt", 25},
		{"file.txt:", "file.txt", 0},
		{"file.txt:invalid", "file.txt", 0},
	}

	for _, test := range tests {
		path, line := parseFileArg(test.input)
		if path != test.path {
			t.Errorf("parseFileArg(%q) path failed. Expected: %q, Got: %q", test.input, test.path, path)
		}
		if line != test.line {
			t.Errorf("parseFileArg(%q) line failed. Expected: %d, Got: %d", test.input, test.line, line)
		}
	}
}

func TestHighlightGo(t *testing.T) {
	tests := []struct {
		line     string
		expected int // Number of style spans expected
	}{
		{"func main() {}", 2}, // func keyword and function name
		{"var x int", 1},      // keyword
		{"\"string\"", 1},     // string literal
		{"// comment", 1},     // comment
		{"123", 1},            // number
		{"x + y", 0},          // no highlighting
	}

	for _, test := range tests {
		spans := highlightGo([]rune(test.line))
		if len(spans) != test.expected {
			t.Errorf("highlightGo(%q) expected %d spans, got %d", test.line, test.expected, len(spans))
		}
	}
}

func TestHighlightMarkdown(t *testing.T) {
	tests := []struct {
		line     string
		expected int // Number of style spans expected
	}{
		{"# Header", 2},    // header (# symbols + rest of line)
		{"- List item", 1}, // list
		{"`code`", 1},      // inline code
		{"**bold**", 1},    // bold
		{"*italic*", 1},    // italic
		{"[link](url)", 1}, // link
		{"normal text", 0}, // no highlighting
	}

	for _, test := range tests {
		spans := highlightMarkdown([]rune(test.line))
		if len(spans) != test.expected {
			t.Errorf("highlightMarkdown(%q) expected %d spans, got %d", test.line, test.expected, len(spans))
		}
	}
}
