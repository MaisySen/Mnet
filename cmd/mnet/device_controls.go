package main

// Local read-only window controls. They never select a peer or grant access.
// Focus and a pending activation are scoped to the current enabled controls.
type deviceControls struct {
	focus      int
	pressed    int
	key        uint32
	mouse      bool
	generation uint64
}

func newDeviceControls() deviceControls                         { return deviceControls{focus: -1, pressed: -1} }
func (c *deviceControls) cancel()                               { c.pressed = -1; c.key = 0; c.mouse = false }
func (c *deviceControls) advance(reverse bool, enabled [4]bool) { c.advanceAmong(reverse, enabled[:]) }
func (c *deviceControls) advanceAmong(reverse bool, enabled []bool) {
	c.cancel()
	step := 1
	if reverse {
		step = -1
		if c.focus < 0 {
			c.focus = 0
		}
	}
	for n := 0; n < len(enabled); n++ {
		c.focus = (c.focus + step + len(enabled)) % len(enabled)
		if enabled[c.focus] {
			return
		}
	}
	c.focus = -1
}
func (c *deviceControls) update(enabled [4]bool, generation uint64) {
	if c.generation != generation || c.pressed >= 0 && !enabled[c.pressed] {
		c.cancel()
	}
	c.generation = generation
	if c.focus >= 0 && !enabled[c.focus] {
		c.focus = -1
	}
}
func (c *deviceControls) pressKey(key uint32, enabled [4]bool) { c.pressKeyAmong(key, enabled[:]) }
func (c *deviceControls) pressKeyAmong(key uint32, enabled []bool) {
	c.cancel()
	if (key == 0xff0d || key == 0x20) && c.focus >= 0 && c.focus < len(enabled) && enabled[c.focus] {
		c.pressed = c.focus
		c.key = key
	}
}
func (c *deviceControls) releaseKey(key uint32, enabled [4]bool) int {
	return c.releaseKeyAmong(key, enabled[:])
}
func (c *deviceControls) releaseKeyAmong(key uint32, enabled []bool) int {
	pressed, held := c.pressed, c.key
	c.cancel()
	if held == 0 || held != key || pressed < 0 || pressed >= len(enabled) || pressed != c.focus || !enabled[pressed] {
		return -1
	}
	return pressed
}
func deviceControlEnabled(loading bool, state string, page, peers int) [4]bool {
	return [4]bool{!loading, state == "up" && page > 0, state == "up" && (page+1)*devicePageSize < peers, true}
}

// Fixed 44px control geometry, shared by press/release hit checks.
func deviceControlHit(button byte, x, y int) int {
	if button != 1 || y < 448 || y >= 492 {
		return -1
	}
	for i, left := range []int{20, 240, 460, 680} {
		if x >= left && x < left+200 {
			return i
		}
	}
	return -1
}
func (c *deviceControls) pressMouse(button byte, x, y int, enabled [4]bool) {
	c.cancel()
	index := deviceControlHit(button, x, y)
	if index >= 0 && enabled[index] {
		c.pressed = index
		c.mouse = true
	}
}
func (c *deviceControls) releaseMouse(button byte, x, y int, enabled [4]bool) int {
	pressed, mouse := c.pressed, c.mouse
	c.cancel()
	if !mouse || pressed < 0 || deviceControlHit(button, x, y) != pressed || !enabled[pressed] {
		return -1
	}
	return pressed
}
