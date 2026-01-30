package ui

import (
	"math"
)

// decorator wraps an Element and modifies its layout and rendering.
// It can act as a marker (e.g., grow) or wrapper (e.g., padding, border, frame).
type decorator struct {
	Element
	grow                   int
	padT, padB, padL, padR int
	width, height          int
	border                 bool
}

func (d decorator) MinSize() (w, h int) {
	mw, mh := d.Element.MinSize()

	mw += d.padL + d.padR
	mh += d.padT + d.padB

	if d.border {
		mw += 2
		mh += 2
	}

	// respect Frame constraints
	if d.width > 0 && mw < d.width {
		mw = d.width
	}
	if d.height > 0 && mh < d.height {
		mh = d.height
	}

	return mw, mh
}

func (d decorator) Layout(r Rect) *Node {
	ix, iy, iw, ih := r.X, r.Y, r.W, r.H

	if d.border {
		ix, iy, iw, ih = ix+1, iy+1, iw-2, ih-2
	}

	ix += d.padL
	iy += d.padT
	iw -= (d.padL + d.padR)
	ih -= (d.padT + d.padB)

	// respect Frame constraints
	if d.width > 0 && iw > d.width {
		iw = d.width
	}
	if d.height > 0 && ih > d.height {
		ih = d.height
	}

	node := &Node{Element: d, Rect: r}
	node.Children = []*Node{d.Element.Layout(Rect{X: ix, Y: iy, W: iw, H: ih})}
	return node
}

func (d decorator) Render(screen *Screen, rect Rect) {
	if d.border {
		drawBorder(screen, rect)
	}
}

// Add FocusTarget to decorator to enable focus delegation through decorators
func (d decorator) FocusTarget() Element {
	return d.Element
}

func (d decorator) OnFocus() {
	if f, ok := d.Element.(Focusable); ok {
		f.OnFocus()
	}
}

func (d decorator) OnBlur() {
	if f, ok := d.Element.(Focusable); ok {
		f.OnBlur()
	}
}

// get or build decorator
func getDecorator(e Element) decorator {
	if d, ok := e.(decorator); ok {
		return d
	}
	return decorator{Element: e}
}

// Pad adds spaces around the element
func Pad(e Element, amount int) Element {
	// Current implementation merges decorator,
	// does not distint inner/outer padding.
	// If needed, allow nesting decorator.
	d := getDecorator(e)
	d.padT, d.padB, d.padL, d.padR = amount, amount, amount, amount
	return d
}

func PadH(e Element, amount int) Element {
	d := getDecorator(e)
	d.padL, d.padR = amount, amount
	return d
}

func PadV(e Element, amount int) Element {
	d := getDecorator(e)
	d.padT, d.padB = amount, amount
	return d
}

// Spacer fills the remaining space between siblings inside a layout container.
var Spacer = Grow(empty{})

// Grow makes the element expand to fill available space in layout container.
func Grow(e Element) Element {
	d := getDecorator(e)
	d.grow = 1
	return d
}

// Frame sets the exact width and height of the element.
func Frame(e Element, w, h int) Element {
	d := getDecorator(e)
	d.width, d.height = w, h
	return d
}

// Border adds a border around the element.
func Border(e Element) Element {
	d := getDecorator(e)
	d.border = true
	return d
}

// vstack is a vertical layout container.
// Itself does not apply any visual styling like background colors, borders,
// it is completely transparent and invisible
type vstack struct {
	children []Element
	spacing  int
}

// VStack arranges children vertically.
func VStack(children ...Element) *vstack {
	v := &vstack{children: children}
	return v
}

func (v *vstack) MinSize() (int, int) {
	maxW, totalH := 0, 0
	for i, child := range v.children {
		cw, ch := child.MinSize()
		if cw > maxW {
			maxW = cw
		}
		totalH += ch
		if i < len(v.children)-1 {
			totalH += v.spacing
		}
	}
	return maxW, totalH
}

func (v *vstack) Layout(r Rect) *Node {
	n := &Node{Element: v, Rect: r}
	n.Children = make([]*Node, 0, len(v.children))
	// First pass: measure children
	totalH := 0
	totalGrow := 0
	for _, child := range v.children {
		if d, ok := child.(decorator); ok && d.grow > 0 {
			totalGrow += d.grow
		} else {
			_, ch := child.MinSize()
			totalH += ch
		}
	}

	// Compute spare space
	spare := max(r.H-totalH-v.spacing*(len(v.children)-1), 0)
	var share float64
	if totalGrow > 0 {
		share = float64(spare) / float64(totalGrow)
	}

	// Second pass: layout children
	used := 0
	for i, child := range v.children {
		if d, ok := child.(*Divider); ok {
			d.vertical = false
		}
		_, ch := child.MinSize()
		if d, ok := child.(decorator); ok && d.grow > 0 {
			expand := int(math.Ceil(float64(d.grow) * share))
			if expand > spare {
				expand = spare
			}
			ch = expand
			spare -= expand
		}
		if used+ch > r.H {
			ch = r.H - used
		}
		if ch > 0 {
			childNode := child.Layout(Rect{X: r.X, Y: r.Y + used, W: r.W, H: ch})
			n.Children = append(n.Children, childNode)
		}
		used += ch
		if i < len(v.children)-1 {
			used += v.spacing
		}
	}
	return n
}

func (v *vstack) Render(s *Screen, rect Rect) {
	// no-op
}

func (v *vstack) Append(e ...Element) *vstack {
	v.children = append(v.children, e...)
	return v
}

// Spacing sets the spacing (in rows) between child elements.
func (v *vstack) Spacing(p int) *vstack {
	v.spacing = p
	return v
}

// hstack is a horizontal layout container.
// Itself does not apply any visual styling like background colors, borders,
// it is completely transparent and invisible
type hstack struct {
	children []Element
	spacing  int
}

// HStack arranges children horizontally.
func HStack(children ...Element) *hstack {
	h := &hstack{children: children}
	return h
}

func (hs *hstack) MinSize() (int, int) {
	totalW, maxH := 0, 0
	for i, child := range hs.children {
		cw, ch := child.MinSize()
		totalW += cw
		if ch > maxH {
			maxH = ch
		}
		if i < len(hs.children)-1 {
			totalW += hs.spacing
		}
	}
	return totalW, maxH
}

func (hs *hstack) Layout(r Rect) *Node {
	n := &Node{Element: hs, Rect: r}
	n.Children = make([]*Node, 0, len(hs.children))
	// First pass: measure children
	totalWidth := 0
	totalGrow := 0
	for _, child := range hs.children {
		if d, ok := child.(decorator); ok && d.grow > 0 {
			totalGrow += d.grow
		} else {
			cw, _ := child.MinSize()
			totalWidth += cw
		}
	}

	// Compute remaining space
	remain := max(r.W-totalWidth-hs.spacing*(len(hs.children)-1), 0)
	var share float64
	if totalGrow > 0 {
		share = float64(remain) / float64(totalGrow)
	}

	// Second pass: layout children
	used := 0
	for i, child := range hs.children {
		if div, ok := child.(*Divider); ok {
			div.vertical = true
		}
		cw, _ := child.MinSize()
		if d, ok := child.(decorator); ok && d.grow > 0 {
			expand := min(int(math.Ceil(float64(d.grow)*share)), remain)
			cw = expand
			remain -= expand
		}
		if used+cw > r.W {
			cw = r.W - used
		}
		if cw > 0 {
			childNode := child.Layout(Rect{X: r.X + used, Y: r.Y, W: cw, H: r.H})
			n.Children = append(n.Children, childNode)
		}
		used += cw
		if i < len(hs.children)-1 {
			used += hs.spacing
		}
	}
	return n
}

func (hs *hstack) Render(s *Screen, rect Rect) {
	// no-op
}

func (hs *hstack) Append(e ...Element) *hstack {
	hs.children = append(hs.children, e...)
	return hs
}

// Spacing sets the spacing (in columns) between child elements.
func (hs *hstack) Spacing(p int) *hstack { hs.spacing = p; return hs }

const (
	hLine          = '─'
	vLine          = '│'
	cornerTopLeft  = '┌'
	cornerTopRight = '┐'
	cornerBotLeft  = '└'
	cornerBotRight = '┘'
)

func drawBorder(s *Screen, rect Rect) {
	// Too small to draw a border
	if rect.W < 2 || rect.H < 2 {
		return
	}

	st := Style{FG: Theme.Border}.Apply()
	// Top and bottom borders
	for i := range rect.W {
		s.SetContent(rect.X+i, rect.Y, hLine, nil, st)
		s.SetContent(rect.X+i, rect.Y+rect.H-1, hLine, nil, st)
	}
	// Left and right borders
	for i := range rect.H {
		s.SetContent(rect.X, rect.Y+i, vLine, nil, st)
		s.SetContent(rect.X+rect.W-1, rect.Y+i, vLine, nil, st)
	}
	// Corners
	s.SetContent(rect.X, rect.Y, cornerTopLeft, nil, st)
	s.SetContent(rect.X+rect.W-1, rect.Y, cornerTopRight, nil, st)
	s.SetContent(rect.X, rect.Y+rect.H-1, cornerBotLeft, nil, st)
	s.SetContent(rect.X+rect.W-1, rect.Y+rect.H-1, cornerBotRight, nil, st)
}

// ResetRect resets the content of the given rectangle to the specified style.
func ResetRect(s *Screen, rect Rect, style Style) {
	st := style.Apply()
	for x := rect.X; x < rect.X+rect.W; x++ {
		for y := rect.Y; y < rect.Y+rect.H; y++ {
			// when debugging, printing '.' would be better
			s.SetContent(x, y, ' ', nil, st)
		}
	}
}

// overlay is a transient container that displays a child element
// over the existing content, typically used for modals or pop-ups.
type overlay struct {
	child     Element
	align     string
	prevFocus Element
}

func (o *overlay) MinSize() (int, int) {
	return o.child.MinSize()
}

func (o *overlay) Layout(r Rect) *Node {
	cw, ch := o.child.MinSize()
	x, y := r.X, r.Y
	switch o.align {
	case "center":
		x = x + (r.W-cw)/2
		y = y + (r.H-ch)/2
	case "top":
		x = x + (r.W-cw)/2
		y = 1 // Small offset from top
	}

	node := &Node{Element: o, Rect: Rect{X: x, Y: y, W: cw, H: ch}}
	node.Children = []*Node{o.child.Layout(Rect{X: x, Y: y, W: cw, H: ch})}
	return node
}

func (o *overlay) Render(s *Screen, rect Rect) {
	ResetRect(s, rect, Style{})
}

/*
type TabItem struct {
	t     *TabView
	label string
	body  Element
}

func (ti *TabItem) OnMouseUp(rx, ry int) {}
func (ti *TabItem) OnMouseDown(rx, ry int) {
	for i, item := range ti.t.items {
		if item == ti {
			ti.t.SetActive(i)
			return
		}
	}
}

func (ti *TabItem) MinSize() (int, int) {
	return runewidth.StringWidth(ti.label), 1
}

func (ti *TabItem) Layout(r Rect) *LayoutNode {
	return &LayoutNode{
		Element: ti,
		Rect:    r,
	}
}

func (ti *TabItem) Render(s *Screen, rect Rect) {
	var st Style
	if ti == ti.t.items[ti.t.active] {
		st.FontUnderline = true
	}
	DrawString(s, rect.X, rect.Y, rect.W, ti.label, st.Apply())
}

func (ti *TabItem) FocusTarget() Element {
	return ti.body
}

func (ti *TabItem) OnFocus() {}
func (ti *TabItem) OnBlur()  {}

type TabView struct {
	items  []*TabItem
	active int
}

func NewTabView() *TabView {
	t := &TabView{}
	return t
}

func (t *TabView) Append(label string, e Element) *TabView {
	t.items = append(t.items, &TabItem{
		t:     t,
		label: label,
		body:  e,
	})
	return t
}

func (t *TabView) SetActive(i int) {
	if i >= 0 && i < len(t.items) {
		t.active = i
	}
}

func (t *TabView) MinSize() (int, int) {
	maxW, maxH := 0, 0
	for _, item := range t.items {
		w, h := item.body.MinSize()
		if w > maxW {
			maxW = w
		}
		if h > maxH {
			maxH = h
		}
	}
	return maxW, maxH + 1 // +1 for tab labels
}

func (t *TabView) Layout(r Rect) *LayoutNode {
	n := &LayoutNode{
		Element: t,
		Rect:    r,
	}

	hs := HStack().Spacing(1)
	for i, item := range t.items {
		hs.Append(item)
		if i != len(t.items)-1 {
			hs.Append(&Divider{})
		}
	}
	n.Children = append(n.Children, hs.Layout(Rect{X: r.X, Y: r.Y, W: r.W, H: 1}))

	if t.active >= 0 && t.active < len(t.items) {
		node := t.items[t.active].body.Layout(Rect{X: r.X, Y: r.Y + 1, W: r.W, H: r.H - 1})
		n.Children = append(n.Children, node)
	}
	return n
}

func (t *TabView) Render(s *Screen, rect Rect) {
	// do nothing, children render themselves
}

func (t *TabView) FocusTarget() Element {
	if t.active < 0 || t.active >= len(t.items) {
		return t
	}
	return t.items[t.active]
}

func (t *TabView) OnFocus() {}
func (t *TabView) OnBlur()  {}
*/
