package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"

	"golang.org/x/term"
)

// Event types
type Event interface {
	isEvent()
}

type EventKey struct {
	key  Key
	ch   rune
	mod  ModMask
	name string
}

func (e *EventKey) isEvent()           {}
func (e *EventKey) Key() Key           { return e.key }
func (e *EventKey) Rune() rune         { return e.ch }
func (e *EventKey) Modifiers() ModMask { return e.mod }
func (e *EventKey) Name() string {
	if e.name != "" {
		return e.name
	}
	if e.ch != 0 {
		return string(e.ch)
	}
	return e.key.String()
}

type EventMouse struct {
	x, y    int
	buttons ButtonMask
}

func (e *EventMouse) isEvent()             {}
func (e *EventMouse) Position() (int, int) { return e.x, e.y }
func (e *EventMouse) Buttons() ButtonMask  { return e.buttons }

type EventResize struct {
	w, h int
}

func (e *EventResize) isEvent()         {}
func (e *EventResize) Size() (int, int) { return e.w, e.h }

type EventInterrupt struct {
	data any
}

func (e *EventInterrupt) isEvent() {}

func NewEventInterrupt(data any) *EventInterrupt {
	return &EventInterrupt{data: data}
}

// Key codes
type Key int16

const (
	KeyRune Key = iota + 256
	KeyUp
	KeyDown
	KeyRight
	KeyLeft
	KeyHome
	KeyEnd
	KeyEnter
	KeyBackspace
	KeyBackspace2
	KeyTAB
	KeyESC
	KeyCtrlA
	KeyCtrlB
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlF
	KeyCtrlG
	KeyCtrlH
	KeyCtrlI
	KeyCtrlJ
	KeyCtrlK
	KeyCtrlL
	KeyCtrlM
	KeyCtrlN
	KeyCtrlO
	KeyCtrlP
	KeyCtrlQ
	KeyCtrlR
	KeyCtrlS
	KeyCtrlT
	KeyCtrlU
	KeyCtrlV
	KeyCtrlW
	KeyCtrlX
	KeyCtrlY
	KeyCtrlZ
)

func (k Key) String() string {
	switch k {
	case KeyUp:
		return "Up"
	case KeyDown:
		return "Down"
	case KeyLeft:
		return "Left"
	case KeyRight:
		return "Right"
	case KeyHome:
		return "Home"
	case KeyEnd:
		return "End"
	case KeyEnter:
		return "Enter"
	case KeyBackspace, KeyBackspace2:
		return "Backspace"
	case KeyTAB:
		return "Tab"
	case KeyESC:
		return "Esc"
	case KeyCtrlC:
		return "Ctrl+C"
	case KeyCtrlP:
		return "Ctrl+P"
	case KeyCtrlN:
		return "Ctrl+N"
	default:
		return fmt.Sprintf("Key(%d)", k)
	}
}

// Modifiers
type ModMask int16

const (
	ModShift ModMask = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
)

// Mouse buttons
type ButtonMask int16

const (
	ButtonNone ButtonMask = iota
	ButtonPrimary
	WheelUp
	WheelDown
)

// Color and Style
type TermStyle struct {
	fg, bg Color
	attrs  AttrMask
}

type AttrMask int32

const (
	AttrBold AttrMask = 1 << iota
	AttrItalic
	AttrUnderline
)

var StyleDefault = TermStyle{fg: ColorWhite, bg: ColorBlack}

func (s TermStyle) Foreground(c Color) TermStyle {
	s.fg = c
	return s
}

func (s TermStyle) Background(c Color) TermStyle {
	s.bg = c
	return s
}

func (s TermStyle) Bold(v bool) TermStyle {
	if v {
		s.attrs |= AttrBold
	} else {
		s.attrs &^= AttrBold
	}
	return s
}

func (s TermStyle) Italic(v bool) TermStyle {
	if v {
		s.attrs |= AttrItalic
	} else {
		s.attrs &^= AttrItalic
	}
	return s
}

func (s TermStyle) Underline(v bool) TermStyle {
	if v {
		s.attrs |= AttrUnderline
	} else {
		s.attrs &^= AttrUnderline
	}
	return s
}

// Color represents an RGB color.
type Color struct {
	r, g, b int
}

// Basic color palette
var (
	ColorBlack   = Color{r: 0, g: 0, b: 0}
	ColorRed     = Color{r: 255, g: 0, b: 0}
	ColorGreen   = Color{r: 0, g: 255, b: 0}
	ColorYellow  = Color{r: 255, g: 255, b: 0}
	ColorBlue    = Color{r: 0, g: 0, b: 255}
	ColorMagenta = Color{r: 255, g: 0, b: 255}
	ColorCyan    = Color{r: 0, g: 255, b: 255}
	ColorWhite   = Color{r: 255, g: 255, b: 255}
	ColorDefault = ColorWhite
)

// GetColor parses color strings (hex or names)
func GetColor(s string) Color {
	if s == "" {
		return ColorDefault
	}
	// Map common color names
	switch strings.ToLower(s) {
	case "black":
		return ColorBlack
	case "red":
		return ColorRed
	case "green":
		return ColorGreen
	case "yellow":
		return ColorYellow
	case "blue":
		return ColorBlue
	case "magenta":
		return ColorMagenta
	case "cyan":
		return ColorCyan
	case "white":
		return ColorWhite
	case "silver":
		return Color{r: 192, g: 192, b: 192}
	default:
		// For hex colors like #RRGGBB
		return parseHexColor(s)
	}
}

func parseHexColor(s string) Color {
	if len(s) == 0 {
		return ColorDefault
	}
	if s[0] == '#' {
		s = s[1:]
	}
	if len(s) != 6 {
		return ColorDefault
	}

	var r, g, b int
	fmt.Sscanf(s[0:2], "%x", &r)
	fmt.Sscanf(s[2:4], "%x", &g)
	fmt.Sscanf(s[4:6], "%x", &b)

	return Color{r: r, g: g, b: b}
}

// Screen manages the terminal screen, handling rendering and input events.
type Screen struct {
	w, h      int
	cells     []cell
	cursorX   int
	cursorY   int
	cursorVis bool
	oldState  *term.State
	eventCh   chan Event
	quit      chan struct{}
	mu        sync.Mutex
	lastBtn   ButtonMask
}

type cell struct {
	ch    rune
	style TermStyle
}

func NewScreen() (*Screen, error) {
	s := &Screen{
		eventCh: make(chan Event, 16),
		quit:    make(chan struct{}),
	}
	return s, nil
}

func (s *Screen) Init() error {
	// Put terminal in raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	s.oldState = oldState

	// Get terminal size
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		term.Restore(int(os.Stdin.Fd()), oldState)
		return err
	}
	s.w, s.h = w, h
	s.cells = make([]cell, w*h)

	// Enter alternate screen buffer, clear, hide cursor
	fmt.Print("\033[?1049h\033[2J\033[H\033[?25l")
	os.Stdout.Sync()

	// Start input reader
	go s.readInput()

	return nil
}

func (s *Screen) Fini() {
	close(s.quit)
	if s.oldState != nil {
		term.Restore(int(os.Stdin.Fd()), s.oldState)
	}
	// Reset cursor to default (blinking block, default color)
	fmt.Print("\033[0 q\033]112\007")
	// Exit alternate screen buffer, show cursor
	fmt.Print("\033[?25h\033[?1049l")
	os.Stdout.Sync()
}

func (s *Screen) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cells {
		s.cells[i] = cell{ch: ' ', style: StyleDefault}
	}
}

func (s *Screen) Fill(ch rune, style TermStyle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cells {
		s.cells[i] = cell{ch: ch, style: style}
	}
}

func (s *Screen) SetContent(x, y int, primary rune, combining []rune, style TermStyle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x < 0 || y < 0 || x >= s.w || y >= s.h {
		return
	}
	s.cells[y*s.w+x] = cell{ch: primary, style: style}
}

func (s *Screen) GetContent(x, y int) (rune, []rune, TermStyle, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x < 0 || y < 0 || x >= s.w || y >= s.h {
		return 0, nil, StyleDefault, 0
	}
	c := s.cells[y*s.w+x]
	return c.ch, nil, c.style, 1
}

// CursorStyle combines cursor shape and blink mode
type CursorStyle int

const (
	CursorBlinkingBlock     CursorStyle = 1 // Blinking block (default)
	CursorSteadyBlock       CursorStyle = 2 // Steady block
	CursorBlinkingUnderline CursorStyle = 3 // Blinking underline
	CursorSteadyUnderline   CursorStyle = 4 // Steady underline
	CursorBlinkingBar       CursorStyle = 5 // Blinking vertical bar
	CursorSteadyBar         CursorStyle = 6 // Steady vertical bar
)

// SetCursorStyle sets the cursor shape/blink and optionally color.
// If no color is provided, only shape/blink is set.
func (s *Screen) SetCursorStyle(style CursorStyle, color ...Color) {
	// Set shape and blink using DECSCUSR (CSI Ps SP q)
	fmt.Printf("\033[%d q", style)

	// Set color if provided using OSC 12
	if len(color) > 0 {
		c := color[0]
		fmt.Printf("\033]12;#%02x%02x%02x\007", c.r, c.g, c.b)
	}
}

func (s *Screen) ShowCursor(x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursorX = x
	s.cursorY = y
	s.cursorVis = true
}

func (s *Screen) HideCursor() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursorVis = false
}

func (s *Screen) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w, s.h
}

func (s *Screen) PollEvent() Event {
	return <-s.eventCh
}

func (s *Screen) PostEvent(ev Event) {
	select {
	case s.eventCh <- ev:
	default:
	}
}

func (s *Screen) Sync() {
	// Full screen redraw on every sync.
	//
	// Trade-offs:
	// - Simple: No dirty tracking, double buffering, or damage regions needed
	// - Predictable: Always correct, no state management bugs
	// - Fast enough: Modern terminals handle 50-200KB ANSI writes in <1ms
	// - Optimized: Style caching (line 433) avoids redundant ANSI codes
	//
	// Alternatives (if profiling shows this is a bottleneck):
	// - Dirty cell tracking: Mark changed cells, only redraw those
	// - Double buffering: Compare old vs new cells, output diffs
	// - Damage regions: Track rectangular areas that changed
	//
	// For typical editor use: Bottleneck is layout computation, not rendering.

	s.mu.Lock()
	defer s.mu.Unlock()

	// Redraw entire screen
	var buf strings.Builder

	lastStyle := StyleDefault
	for y := 0; y < s.h; y++ {
		// Position cursor at the start of each line explicitly
		fmt.Fprintf(&buf, "\033[%d;1H", y+1)

		for x := 0; x < s.w; x++ {
			c := s.cells[y*s.w+x]
			if c.style != lastStyle {
				buf.WriteString(s.styleToANSI(c.style))
				lastStyle = c.style
			}
			if c.ch == 0 {
				buf.WriteRune(' ')
			} else {
				buf.WriteRune(c.ch)
			}
		}
	}
	buf.WriteString("\033[0m") // Reset style

	// Position cursor
	if s.cursorVis {
		fmt.Fprintf(&buf, "\033[%d;%dH\033[?25h", s.cursorY+1, s.cursorX+1)
	} else {
		buf.WriteString("\033[?25l")
	}

	os.Stdout.WriteString(buf.String())
	os.Stdout.Sync()
}

func (s *Screen) EnableMouse() {
	// Enable mouse tracking modes:
	// ?1000h = button press/release tracking
	// ?1002h = button motion tracking (motion while button pressed)
	// ?1006h = SGR mouse mode (extended coordinates)
	//
	// Note: We use ?1002h instead of ?1003h (all motion tracking) because:
	// - ?1003h generates excessive motion events that can overwhelm the terminal
	// - Motion event flood causes input parsing issues and raw escape sequences leaking as text
	// - ?1002h provides stable drag selection without the event deluge
	// - Trade-off: No hover feedback on UI elements (buttons/tabs), only press/drag feedback
	fmt.Print("\033[?1000h\033[?1002h\033[?1006h")
	os.Stdout.Sync()
}

func (s *Screen) SetClipboard(data []byte) {
	// OSC 52 clipboard escape sequence
	// This is a simple implementation that works on many terminals
	text := string(data)
	encoded := strings.ReplaceAll(text, "\n", "\r\n")
	fmt.Printf("\033]52;c;%s\007", encoded)
	os.Stdout.Sync()
}

func (s *Screen) styleToANSI(st TermStyle) string {
	var codes []string

	// Always start with reset to clear previous attributes
	codes = append(codes, "0")

	// Foreground color - use truecolor (24-bit RGB)
	codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", st.fg.r, st.fg.g, st.fg.b))

	// Background color - use truecolor (24-bit RGB)
	codes = append(codes, fmt.Sprintf("48;2;%d;%d;%d", st.bg.r, st.bg.g, st.bg.b))

	// Attributes (applied after reset and colors)
	if st.attrs&AttrBold != 0 {
		codes = append(codes, "1")
	}
	if st.attrs&AttrItalic != 0 {
		codes = append(codes, "3")
	}
	if st.attrs&AttrUnderline != 0 {
		codes = append(codes, "4")
	}

	return "\033[" + strings.Join(codes, ";") + "m"
}

func (s *Screen) readInput() {
	buf := make([]byte, 256)
	for {
		select {
		case <-s.quit:
			return
		default:
		}

		// Set read timeout using syscall
		tv := syscall.Timeval{Sec: 0, Usec: 100000} // 100ms timeout
		syscall.Select(int(os.Stdin.Fd())+1, &syscall.FdSet{}, nil, nil, &tv)

		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		s.parseInput(buf[:n])
	}
}

func (s *Screen) parseInput(buf []byte) {
	i := 0
	for i < len(buf) {
		// ESC sequence
		if buf[i] == 0x1b {
			if i+1 < len(buf) && buf[i+1] == '[' {
				// CSI sequence
				ev, consumed := s.parseCSI(buf[i:])
				if ev != nil {
					s.PostEvent(ev)
				}
				i += consumed
				continue
			} else {
				// Plain ESC
				s.PostEvent(&EventKey{key: KeyESC, name: "Esc"})
				i++
				continue
			}
		}

		// Control characters
		if buf[i] < 32 {
			s.PostEvent(s.parseControl(buf[i]))
			i++
			continue
		}

		// Regular UTF-8 character
		r, size := utf8.DecodeRune(buf[i:])
		if r != utf8.RuneError {
			s.PostEvent(&EventKey{key: KeyRune, ch: r})
			i += size
		} else {
			i++
		}
	}
}

func (s *Screen) parseControl(b byte) *EventKey {
	switch b {
	case 0x08, 0x7f: // BS, DEL
		return &EventKey{key: KeyBackspace, name: "Backspace"}
	case 0x09: // TAB
		return &EventKey{key: KeyTAB, name: "Tab"}
	case 0x0d, 0x0a: // CR, LF
		return &EventKey{key: KeyEnter, name: "Enter"}
	case 0x01: // Ctrl+A
		return &EventKey{key: KeyCtrlA, mod: ModCtrl, name: "Ctrl+A"}
	case 0x02: // Ctrl+B
		return &EventKey{key: KeyCtrlB, mod: ModCtrl, name: "Ctrl+B"}
	case 0x03: // Ctrl+C
		return &EventKey{key: KeyCtrlC, mod: ModCtrl, name: "Ctrl+C"}
	case 0x04: // Ctrl+D
		return &EventKey{key: KeyCtrlD, mod: ModCtrl, name: "Ctrl+D"}
	case 0x05: // Ctrl+E
		return &EventKey{key: KeyCtrlE, mod: ModCtrl, name: "Ctrl+E"}
	case 0x06: // Ctrl+F
		return &EventKey{key: KeyCtrlF, mod: ModCtrl, name: "Ctrl+F"}
	case 0x07: // Ctrl+G
		return &EventKey{key: KeyCtrlG, mod: ModCtrl, name: "Ctrl+G"}
	case 0x0b: // Ctrl+K
		return &EventKey{key: KeyCtrlK, mod: ModCtrl, name: "Ctrl+K"}
	case 0x0c: // Ctrl+L
		return &EventKey{key: KeyCtrlL, mod: ModCtrl, name: "Ctrl+L"}
	case 0x0e: // Ctrl+N
		return &EventKey{key: KeyCtrlN, mod: ModCtrl, name: "Ctrl+N"}
	case 0x0f: // Ctrl+O
		return &EventKey{key: KeyCtrlO, mod: ModCtrl, name: "Ctrl+O"}
	case 0x10: // Ctrl+P
		return &EventKey{key: KeyCtrlP, mod: ModCtrl, name: "Ctrl+P"}
	case 0x11: // Ctrl+Q
		return &EventKey{key: KeyCtrlQ, mod: ModCtrl, name: "Ctrl+Q"}
	case 0x12: // Ctrl+R
		return &EventKey{key: KeyCtrlR, mod: ModCtrl, name: "Ctrl+R"}
	case 0x13: // Ctrl+S
		return &EventKey{key: KeyCtrlS, mod: ModCtrl, name: "Ctrl+S"}
	case 0x14: // Ctrl+T
		return &EventKey{key: KeyCtrlT, mod: ModCtrl, name: "Ctrl+T"}
	case 0x15: // Ctrl+U
		return &EventKey{key: KeyCtrlU, mod: ModCtrl, name: "Ctrl+U"}
	case 0x16: // Ctrl+V
		return &EventKey{key: KeyCtrlV, mod: ModCtrl, name: "Ctrl+V"}
	case 0x17: // Ctrl+W
		return &EventKey{key: KeyCtrlW, mod: ModCtrl, name: "Ctrl+W"}
	case 0x18: // Ctrl+X
		return &EventKey{key: KeyCtrlX, mod: ModCtrl, name: "Ctrl+X"}
	case 0x19: // Ctrl+Y
		return &EventKey{key: KeyCtrlY, mod: ModCtrl, name: "Ctrl+Y"}
	case 0x1a: // Ctrl+Z
		return &EventKey{key: KeyCtrlZ, mod: ModCtrl, name: "Ctrl+Z"}
	default:
		return &EventKey{key: Key(256 + int(b)), mod: ModCtrl}
	}
}

func (s *Screen) parseCSI(buf []byte) (Event, int) {
	if len(buf) < 3 || buf[0] != 0x1b || buf[1] != '[' {
		return nil, 1
	}

	// Find end of CSI sequence
	end := 2
	for end < len(buf) && buf[end] >= 0x20 && buf[end] <= 0x3f {
		end++
	}
	if end >= len(buf) {
		return nil, 1
	}

	final := buf[end]
	end++

	seq := string(buf[2 : end-1])

	// Arrow keys
	switch final {
	case 'A':
		return &EventKey{key: KeyUp, name: "Up"}, end
	case 'B':
		return &EventKey{key: KeyDown, name: "Down"}, end
	case 'C':
		return &EventKey{key: KeyRight, name: "Right"}, end
	case 'D':
		return &EventKey{key: KeyLeft, name: "Left"}, end
	case 'H':
		return &EventKey{key: KeyHome, name: "Home"}, end
	case 'F':
		return &EventKey{key: KeyEnd, name: "End"}, end
	case 'M', 'm': // Mouse events
		ev, consumed := s.parseMouse(seq, final == 'm')
		if ev != nil {
			return ev, end
		}
		return nil, consumed
	}

	// Extended sequences like 1;2A (Shift+Up)
	if strings.Contains(seq, ";") {
		parts := strings.Split(seq, ";")
		if len(parts) >= 2 {
			mod := s.parseModifier(parts[0])
			switch final {
			case 'A':
				return &EventKey{key: KeyUp, mod: mod, name: "Up"}, end
			case 'B':
				return &EventKey{key: KeyDown, mod: mod, name: "Down"}, end
			case 'C':
				return &EventKey{key: KeyRight, mod: mod, name: "Right"}, end
			case 'D':
				return &EventKey{key: KeyLeft, mod: mod, name: "Left"}, end
			}
		}
	}

	return nil, end
}

func (s *Screen) parseModifier(mod string) ModMask {
	var m int
	fmt.Sscanf(mod, "%d", &m)

	var mask ModMask
	if m&1 != 0 {
		mask |= ModShift
	}
	if m&4 != 0 {
		mask |= ModAlt
	}
	if m&8 != 0 {
		mask |= ModMeta
	}
	return mask
}

func (s *Screen) parseMouse(seq string, release bool) (*EventMouse, int) {
	// SGR mouse format: <b;x;y (the < is part of the sequence)
	// Strip the leading < if present
	seq = strings.TrimPrefix(seq, "<")

	parts := strings.Split(seq, ";")
	if len(parts) < 3 {
		return nil, 0
	}

	var btn, x, y int
	fmt.Sscanf(parts[0], "%d", &btn)
	fmt.Sscanf(parts[1], "%d", &x)
	fmt.Sscanf(parts[2], "%d", &y)

	x-- // Convert to 0-based
	y--

	// Mouse button encoding in SGR mode:
	// Base button codes (lower 2 bits):
	//   0 = left, 1 = middle, 2 = right
	// Modifier bits:
	//   4 = Shift, 8 = Meta, 16 = Control
	// Motion bit:
	//   32 = motion while button pressed
	// Wheel bit:
	//   64 = wheel event
	// So wheel up = 64, wheel down = 65

	// Check for wheel events (bit 6 set)
	if btn >= 64 && btn < 128 {
		// This is a wheel event
		if (btn & 1) == 0 {
			// Even number = wheel up
			return &EventMouse{x: x, y: y, buttons: WheelUp}, 0
		} else {
			// Odd number = wheel down
			return &EventMouse{x: x, y: y, buttons: WheelDown}, 0
		}
	}

	// Regular button events
	var button ButtonMask
	if release {
		button = ButtonNone
		s.lastBtn = ButtonNone
	} else {
		// Check for motion with button held (bit 5 set)
		if (btn & 32) != 0 {
			// Motion event - check the base button
			baseBtn := btn & 3
			if baseBtn == 3 {
				// Motion without button (code 35) - pure hover
				button = ButtonNone
			} else {
				// Motion with button held
				button = s.lastBtn
			}
		} else if (btn & 3) == 0 {
			// Left button
			button = ButtonPrimary
			s.lastBtn = ButtonPrimary
		} else {
			button = ButtonNone
		}
	}

	return &EventMouse{x: x, y: y, buttons: button}, 0
}
