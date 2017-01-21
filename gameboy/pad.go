package gameboy

// Gamepad represents the GameBoy input hardware.
type Gamepad struct {
	Down, Up, Left, Right bool
	Start, Select, B, A   bool
}
