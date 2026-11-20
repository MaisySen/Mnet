package desk

// Displays returns a copy of the exact compiled arrangement. It is review
// evidence only, never discovery, paired-device identity or input authority.
func (c *Compiled) Displays() []Display {
	if c == nil {
		return nil
	}
	return append([]Display(nil), c.displays...)
}
