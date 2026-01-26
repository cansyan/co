A text user interface (TUI) package for building interactive terminal applications in Go.

Features:
- element: Button, Text, TextInput, TextViewer, TextEditor, Spacer, Divider
- container: VStack, HStack, Border, Padding, Overlay
- keyboard: focus management, event handling, keybinding
- mouse: hover enter/move/leave, click, scroll

Design decisions:
- tcell does the actual terminal rendering
- redraw on state changes
- child element does not inherit the parent's style
