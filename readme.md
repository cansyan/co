TODO:
[x] hit testing
[ ] event handling
[ ] cursor

A draft of minimal retained-mode layout engine:
```go
type Element interface {
	Measure(availW, availH int) (w, h int)
	Layout(x, y, w, h int) *LayoutNode
	Render(s Screen, rect Rect, style Style)
}

type LayoutNode struct {
	Element  Element
	Rect     Rect
	Children []*LayoutNode
}

type Rect struct {
	X, Y, W, H int
}

func drawTree(node *LayoutNode, s Screen, parent Style) {
	if node == nil {
		return
	}

	node.Element.Render(s, node.Rect, parent)
	for _, child := range node.Children {
		drawTree(child, s, parent)
	}
}

// Example Render() Implementations
func (t *Text) Render(s Screen, rect Rect, parent Style) {
	st := mergeStyle(parent, t.style)
	drawString(s, rect.X, rect.Y, st.TextColor, t.value)
}
func (h *HStack) Render(s Screen, rect Rect, parent Style) {
	// Usually nothing, children are drawn in drawTree()
}

type App struct {
	Root   Element
	Tree   *LayoutNode
	Screen Screen
}

func (a *App) Render() {
	w, h := a.Screen.Size()
	a.Tree = a.Root.Layout(0, 0, w, h)
	drawTree(a.Tree, a.Screen, Style{})
}

func (a *App) HitTest(px, py int) Element {
	return walk(a.Tree, px, py)
}

func walk(node *LayoutNode, px, py int) Element {
	r := node.Rect
	if px < r.X || py < r.Y || px >= r.X+r.W || py >= r.Y+r.H {
		return nil
	}

	// Search children first (topmost)
	for _, c := range node.Children {
		if e := walkHit(c, px, py); e != nil {
			return e
		}
	}
	return node.Element
}
```

