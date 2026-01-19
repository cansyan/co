# co

A lightweight, terminal-based text editor built in Go.

## Features

- multiple tabs
- Undo/Redo
- Copy/Cut/Paste
- Find
- Syntax highlighting
- Automatic formatting
- Automatic indentation
- Color themes
- Goto definition
- Inline suggestion
- Command Palette

## Usage

```bash
go build -o co main.go
./co [filename]
```

## Keyboard Shortcuts

```
File Operations:
    Ctrl+T: New tab
    Ctrl+O: Open file dialog
    Ctrl+S: Save current file
    Ctrl+W: Close current tab (with save prompt)
    Ctrl+Q: Quit editor

Editing:
    Ctrl+Z: Undo
    Ctrl+Y: Redo
    Ctrl+C: Copy selection (copies current line if no selection)
    Ctrl+X: Cut selection (cuts current line if no selection)
    Ctrl+V: Paste
    Ctrl+A: Select all
    Ctrl+L: Expand selection to line
    Ctrl+B: Expand selection to brackets
    Ctrl+D: Select word or find next occurrence
    Tab: Accept inline suggestion, if exists

Search & Navigation:
    Ctrl+F: Open search bar
    Enter / Ctrl+N: Next match
    Ctrl+P / Up: Previous match
    Esc: Close search / Clear selection

Code Navigation:
    Ctrl+G: Go to definition
    Alt+Up: Go to first line
    Alt+Down: Go to last line
    Alt+Left: Go to line start (first non-space character)
    Alt+Right: Go to line end

Command Palette:
    Ctrl+P: Go to file
    Ctrl+R: Go to symbol
    Ctrl+K Ctrl+P: Run command
```

## Command Palette Prefixes
- `:` Go to line number
- `@` Go to symbol
- `>` Run command
