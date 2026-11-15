//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// Text injection modes. Viewers send key events carrying whole text
// strings; how the host turns them into keystrokes is a host-side
// choice, set with MNET_TEXT_MODE:
//
//	auto      (default) keysym typing on X11, ydotool type on Wayland
//	keysym    X11 only: map each rune to a keysym, remapping a spare
//	          keycode for characters outside the current layout
//	clipboard set the clipboard (xclip/xsel on X11, wl-copy on
//	          Wayland) and inject Ctrl+V; handles any text the
//	          clipboard tool accepts, at the cost of clobbering the
//	          user's clipboard
//
// resolveTextMode validates the request against backend and tools,
// falling back to the backend default when a clipboard tool is
// missing.
func resolveTextMode(requested, backend string, haveFn func(string) bool) string {
	def := "keysym"
	tool := "xclip"
	alt := "xsel"
	if backend == "wayland" {
		def = "ydotool"
		tool, alt = "wl-copy", ""
	}
	switch requested {
	case "", "auto":
		return def
	case "clipboard":
		if haveFn(tool) || (alt != "" && haveFn(alt)) {
			return "clipboard"
		}
		return def
	case "keysym", "ydotool":
		return requested
	}
	return def
}

// keysymForRune maps any rune to an X keysym. ASCII keysyms equal
// their rune values; everything else uses the Unicode keysym range
// 0x01000000 | codepoint.
func keysymForRune(r rune) xproto.Keysym {
	if r >= 0x20 && r < 0x7f {
		return xproto.Keysym(r)
	}
	return xproto.Keysym(0x01000000) | xproto.Keysym(r)
}

// TypeText types a whole string on X11: clipboard paste in clipboard
// mode, per-rune keysym typing otherwise.
func (x *x11Host) TypeText(s string) error {
	if x.conn == nil {
		return fmt.Errorf("injector not connected")
	}
	if x.textMode == "clipboard" {
		if err := x.pasteText(s); err == nil {
			return nil
		}
		// Fall through to keysym typing if the clipboard path failed.
	}
	for _, r := range s {
		if err := x.typeRune(r); err != nil {
			return err
		}
	}
	x.conn.Sync()
	return nil
}

// typeRune presses one rune. Characters missing from the current
// keyboard layout are produced by remapping a spare keycode, the same
// technique xdotool uses: the remapped keycode is kept and reused for
// later unmapped characters.
func (x *x11Host) typeRune(r rune) error {
	sym := keysymForRune(r)
	kc := keycodeFor(x.conn, sym)
	if kc == 0 {
		var err error
		kc, err = x.remapKeycode(sym)
		if err != nil {
			return err
		}
	}
	xtest.FakeInput(x.conn, xproto.KeyPress, kc, 0, x.root, 0, 0, 0)
	xtest.FakeInput(x.conn, xproto.KeyRelease, kc, 0, x.root, 0, 0, 0)
	return nil
}

// remapKeycode binds a keysym to a spare keycode (all-NoSymbol slot),
// reusing the previously remapped slot when there is one.
func (x *x11Host) remapKeycode(sym xproto.Keysym) (byte, error) {
	setup := xproto.Setup(x.conn)
	min, max := setup.MinKeycode, setup.MaxKeycode
	mapping, err := xproto.GetKeyboardMapping(x.conn, min, byte(max-min+1)).Reply()
	if err != nil {
		return 0, fmt.Errorf("keyboard mapping: %w", err)
	}
	per := int(mapping.KeysymsPerKeycode)
	kc := x.remapKC
	if kc == 0 {
		for k := int(min); k <= int(max); k++ {
			free := true
			for i := 0; i < per; i++ {
				if mapping.Keysyms[(k-int(min))*per+i] != 0 {
					free = false
					break
				}
			}
			if free {
				kc = byte(k)
				break
			}
		}
		if kc == 0 {
			return 0, fmt.Errorf("no spare keycode for keysym %#x", uint32(sym))
		}
	}
	syms := make([]xproto.Keysym, per)
	syms[0] = sym
	if err := xproto.ChangeKeyboardMapping(x.conn, 1, xproto.Keycode(kc), byte(per), syms).Check(); err != nil {
		return 0, fmt.Errorf("remap keycode %d: %w", kc, err)
	}
	x.conn.Sync()
	x.remapKC = kc
	return kc, nil
}

// pasteText sets the clipboard with xclip or xsel and injects Ctrl+V.
func (x *x11Host) pasteText(s string) error {
	tool, args := "xclip", []string{"-selection", "clipboard"}
	if !have("xclip") {
		if !have("xsel") {
			return fmt.Errorf("no clipboard tool (xclip/xsel)")
		}
		tool, args = "xsel", []string{"--clipboard", "--input"}
	}
	cmd := exec.Command(tool, args...)
	cmd.Stdin = strings.NewReader(s)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}
	ctrl := keycodeFor(x.conn, 0xffe3) // Control_L
	v := keycodeFor(x.conn, 'v')
	if ctrl == 0 || v == 0 {
		return fmt.Errorf("no Ctrl/V keycodes in layout")
	}
	xtest.FakeInput(x.conn, xproto.KeyPress, ctrl, 0, x.root, 0, 0, 0)
	xtest.FakeInput(x.conn, xproto.KeyPress, v, 0, x.root, 0, 0, 0)
	xtest.FakeInput(x.conn, xproto.KeyRelease, v, 0, x.root, 0, 0, 0)
	xtest.FakeInput(x.conn, xproto.KeyRelease, ctrl, 0, x.root, 0, 0, 0)
	x.conn.Sync()
	return nil
}

// TypeText types a whole string on Wayland: wl-copy plus Ctrl+V in
// clipboard mode, ydotool type otherwise.
func (i *waylandInjector) TypeText(s string) error {
	if i.textMode == "clipboard" && have("wl-copy") {
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(s)
		if err := cmd.Run(); err == nil {
			// 29 = LEFTCTRL, 47 = V
			return run("ydotool", "key", "29:1", "47:1", "47:0", "29:0")
		}
	}
	return run("ydotool", "type", "--", s)
}
