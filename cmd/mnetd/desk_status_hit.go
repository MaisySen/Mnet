package main

// One local 44px Stop target. No device/session authority is created here.
func deskStopHit(button byte, x, y int) bool {
	return button == 1 && x >= 18 && x < 702 && y >= 240 && y < 284
}
