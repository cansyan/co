A text user interface (TUI) package for building interactive terminal applications in Go.

Features:
- element: Button, Text, TextInput, TextViewer, TextEditor, Spacer, Divider
- container: VStack, HStack, Border, Padding, Overlay
- keyboard: focus management, event handling, keybinding
- mouse: click, drag selection, scroll

Design decisions:
- Custom terminal implementation using Go's standard library (golang.org/x/term)
- ANSI escape sequences for rendering and input parsing
- Mouse tracking uses ?1002h mode (button motion only) to avoid event flood
- No hover state support: ?1003h mode (all motion) causes excessive events that overwhelm parsing
- Redraw on state changes
- Child element does not inherit the parent's style
