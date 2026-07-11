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
    ctrl+t: new tab
    ctrl+o: open file
    ctrl+s: save file
    ctrl+w: close tab
    ctrl+q: quit

Editing:
    ctrl+z: undo
    ctrl+y: redo
    ctrl+c: copy selection (copies current line if no selection)
    ctrl+x: cut selection (cuts current line if no selection)
    ctrl+v: paste
    ctrl+l: expand selection to line
    ctrl+b: expand selection to brackets
    ctrl+d: select word or find next occurrence
    tab: accept inline suggestion, if exists

Search & Navigation:
    ctrl+f: open search bar
    enter / ctrl+n: Next match
    ctrl+p / up: Previous match
    esc: close search / clear selection

Code Navigation:
    ctrl+g: go to definition
    alt+up: go to first line
    alt+down: go to last line
    ctrl+a / alt+left: go to line start (first non-space character)
    ctrl+e / lt+right: go to line end

Command Palette:
    ctrl+o: go to file
    ctrl+r: go to symbol
    ctrl+p: run command
```

## Command Palette Prefixes
- `:` go to line number
- `@` go to symbol
- `>` run command
