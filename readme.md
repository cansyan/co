A text-based user interface (TUI) package, built on top of tcell.

Features:
- component: Button, Text, TextInput, TextViewer, TextEditor, Spacer, Divider
- container: VStack, HStack, Border, Padding, Overlay
- keyboard: focus management, event handling
  - [x] keybindings
- mouse: hover, click, scroll
  - [ ] drag

To maintain simplicity:
- redraw on every event
- child element do not inherit the container's style
