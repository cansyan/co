A text user interface (TUI) package, built on top of tcell.

Features:
- element: Button, Text, TextInput, TextViewer, TextEditor, Spacer, Divider
- container: VStack, HStack, Border, Padding, Overlay
- keyboard: focus management, event handling, keybinding
- mouse: hover enter/move/leave, click, scroll

Design decisions:
- redraw on every event
- child element does not inherit the parent's style
