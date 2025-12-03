A text-based user interface (TUI) package, built on top of tcell.

Features:
- component: Button, Text, TextInput, TextViewer, TextEditor, Spacer, Divider
- container: VStack, HStack, Box, Padding
- keyboard: focus management, event handling
  - [ ] keybindings
- mouse: hover, click, scroll
  - [ ] drag

To maintain simplicity and clarity:
- redraw on every event
- child element do not inherit the container's style

