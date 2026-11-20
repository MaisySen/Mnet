package main

// Local read-only window controls. They never select a peer or grant access.
// Focus and a pending activation are scoped to the current enabled controls.
type deviceControls struct {
	focus      int
	pressed    int
	key        uint32
	generation uint64
}

func newDeviceControls() deviceControls { return deviceControls{focus: -1, pressed: -1} }
func (c *deviceControls) cancel()       { c.pressed = -1; c.key = 0 }
func (c *deviceControls) advance(reverse bool, enabled [4]bool) {
	c.cancel()
	step := 1
	if reverse {
		step = -1
		if c.focus < 0 {
			c.focus = 0
		}
	}
	for n := 0; n < 4; n++ {
		c.focus = (c.focus + step + 4) % 4
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
func (c *deviceControls) pressKey(key uint32, enabled [4]bool) {
	c.cancel()
	if (key == 0xff0d || key == 0x20) && c.focus >= 0 && enabled[c.focus] {
		c.pressed = c.focus
		c.key = key
	}
}
func (c *deviceControls) releaseKey(key uint32, enabled [4]bool) int {
	pressed, held := c.pressed, c.key
	c.cancel()
	if held != key || pressed < 0 || pressed != c.focus || !enabled[pressed] {
		return -1
	}
	return pressed
}
func deviceControlEnabled(loading bool, state string, page, peers int) [4]bool {
	return [4]bool{!loading, state == "up" && page > 0, state == "up" && (page+1)*devicePageSize < peers, true}
}
