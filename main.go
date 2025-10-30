package main

import (
	"log"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

type Node struct {
	X, Y     int
	Width    int
	Height   int
	Parent   *Node
	Children []*Node
	layout   string // "horizontal" or "vertical"

	Text    string
	Color   tcell.Color
	OnClick func()
}

func (n *Node) Draw(screen tcell.Screen) {
	if len(n.Children) == 0 {
		style := tcell.Style{}.Foreground(n.Color)
		col := 0
		// TODO: mutiple lines
		for _, c := range []rune(n.Text) {
			if col >= n.Width {
				return
			}
			screen.SetContent(n.X+col, n.Y, c, nil, style)
			col += runewidth.RuneWidth(c)
		}
		// Clear remaining space
		for i := col; i < n.Width; i++ {
			screen.SetContent(n.X+i, n.Y, ' ', nil, style)
		}
		return
	}

	for _, child := range n.Children {
		child.Draw(screen)
	}
}

// Resize calculate position and size
func (n *Node) Resize(x, y, width, height int) {
	n.X = x
	n.Y = y
	n.Width = width
	n.Height = height
	if len(n.Children) == 0 {
		return
	}
	switch n.layout {
	case "horizontal":
		width := n.Width / len(n.Children)
		for i, child := range n.Children {
			child.Resize(n.X+i*width, n.Y, width, n.Height)
		}
	case "vertical":
		height := n.Height / len(n.Children)
		for i, child := range n.Children {
			child.Resize(n.X, n.Y+i*height, n.Width, height)
		}
	}
}

func VStack(item ...*Node) *Node {
	n := &Node{
		layout:   "vertical",
		Children: item,
	}
	return n
}

func HStack(item ...*Node) *Node {
	n := &Node{
		layout:   "horizontal",
		Children: item,
	}
	return n
}

func Text(text string, color tcell.Color) *Node {
	return &Node{
		Text:  text,
		Color: color,
	}
}

func Button(text string, onClick func()) *Node {
	return &Node{
		Text:    text,
		OnClick: onClick,
	}
}

func main() {
	// Initialize screen
	screen, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("%+v", err)
	}
	if err := screen.Init(); err != nil {
		log.Fatalf("%+v", err)
	}
	// s.SetStyle(styleBase)
	// s.SetCursorStyle(tcell.CursorStyleBlinkingBar, cursorColor)
	screen.EnableMouse()
	screen.EnablePaste()
	screen.Clear()
	quit := func() {
		err := recover()
		screen.Fini()
		if err != nil {
			panic(err)
		}
	}
	defer quit()

	root := VStack(
		HStack(
			Text("File", tcell.ColorWhite),
			Text("Edit", tcell.ColorWhite),
			Text("View", tcell.ColorWhite),
			Text("Help", tcell.ColorWhite),
		),
		HStack(
			Text("New", tcell.ColorWhite),
			Text("Open", tcell.ColorWhite),
			Text("Save", tcell.ColorWhite),
			Text("Exit", tcell.ColorWhite),
		),
	)

	for {
		screen.Show()
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape:
				return
			}
		case *tcell.EventResize:
			w, h := screen.Size()
			root.Resize(0, 0, w, h)
			root.Draw(screen)
			screen.Sync()
		}
	}
}
