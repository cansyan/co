# co

A lightweight, terminal-based text editor built in Go with syntax highlighting, 
multi-tab support, and powerful navigation features.

## Features

- **Multi-tab Interface** - Work with multiple files simultaneously
- **Syntax Highlighting** - Go syntax highlighting with keyword, string, comment, and function recognition
- **Auto-formatting** - Automatic Go code formatting on save using `gofmt`
- **Smart Search** - Fast incremental search with match navigation
- **Command Palette** - Quick access to files, symbols, and commands
- **Go-to Definition** - Navigate to symbol definitions within the file
- **Auto-completion** - Context-aware completion for symbols and functions
- **Undo/Redo** - Full edit history support
- **Copy/Cut/Paste** - Built-in clipboard operations

## Installation

```bash
go build -o co main.go
```

## Usage

```bash
# Open the editor with a new file
./co

# Open an existing file
./co /path/to/file.go

# Use light color theme
./co -light /path/to/file.go
```

## Keyboard Shortcuts

### File Operations
- `Ctrl+T` - New tab
- `Ctrl+O` - Open file dialog
- `Ctrl+S` - Save current file
- `Ctrl+W` - Close current tab (with save prompt)
- `Ctrl+Q` - Quit editor

### Editing
- `Ctrl+Z` - Undo
- `Ctrl+Y` - Redo
- `Ctrl+C` - Copy selection (copies current line if no selection)
- `Ctrl+X` - Cut selection (cuts current line if no selection)
- `Ctrl+V` - Paste
- `Ctrl+A` - Select all
- `Ctrl+L` - Expand selection to line
- `Ctrl+B` - Expand selection to brackets
- `Ctrl+D` - Select word or find next occurrence

### Search & Navigation
- `Ctrl+F` - Open search bar
- `Enter` / `Ctrl+N` - Next match
- `Ctrl+P` / `Up` - Previous match
- `Esc` - Close search / Clear selection

### Command Palette
- `Ctrl+P` - Open file search
- `Ctrl+K Ctrl+P` - Open command palette
- `Ctrl+R` - Go to symbol (`@` mode)

#### Palette Modes
- **Default** - File search (shows open tabs + files in current directory)
- `:` - Go to line (e.g., `:42`)
- `@` - Go to symbol (search functions and types)
- `>` - Command mode (theme switching, commands)

### Code Navigation
- `Ctrl+G` - Go to definition
- `Ctrl+N` - Auto-complete symbol
- `Alt+Up` - Go to first line
- `Alt+Down` - Go to last line
- `Alt+Left` - Go to first non-space character
- `Alt+Right` - Go to end of line

## Architecture

The editor is built on a custom TUI package (`co/ui`) with:
- Component-based layout system
- Event-driven input handling
- Efficient screen rendering with `tcell`
