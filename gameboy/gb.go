package gameboy

// Machine is... the Nintendo GameBoy.
type Machine struct {
	bus  Bus
	cpu  CPU
	ppu  PPU
	apu  APU
	cart IO
}

// NewMachine creates a new GameBoy machine.
func NewMachine(cart IO, useBootrom bool) *Machine {
	gb := new(Machine)

	// Cartridge
	gb.cart = cart
	for i := 0x0000; i < 0x8000; i++ {
		gb.bus.io[i] = gb.cart
	}

	// Video RAM
	for i := 0x8000; i < 0xA000; i++ {
		gb.bus.io[i] = &gb.ppu
	}

	// External RAM
	for i := 0xA000; i < 0xC000; i++ {
		gb.bus.io[i] = gb.cart
	}

	// Work RAM
	wram := &WRAM{}
	for i := 0xC000; i < 0xFE00; i++ {
		gb.bus.io[i] = wram
	}

	// Sprite attribute table
	for i := 0xFE00; i < 0xFEA0; i++ {
		gb.bus.io[i] = &gb.ppu
	}

	// CPU registers
	gb.bus.io[0xFF00] = &gb.cpu
	gb.bus.io[0xFF01] = &gb.cpu
	gb.bus.io[0xFF02] = &gb.cpu
	gb.bus.io[0xFF04] = &gb.cpu
	gb.bus.io[0xFF05] = &gb.cpu
	gb.bus.io[0xFF06] = &gb.cpu
	gb.bus.io[0xFF07] = &gb.cpu
	gb.bus.io[0xFF0F] = &gb.cpu
	gb.bus.io[0xFF46] = &gb.cpu

	// PPU registers
	gb.bus.io[0xFF40] = &gb.ppu
	gb.bus.io[0xFF41] = &gb.ppu
	gb.bus.io[0xFF42] = &gb.ppu
	gb.bus.io[0xFF43] = &gb.ppu
	gb.bus.io[0xFF44] = &gb.ppu
	gb.bus.io[0xFF45] = &gb.ppu
	gb.bus.io[0xFF47] = &gb.ppu
	gb.bus.io[0xFF48] = &gb.ppu
	gb.bus.io[0xFF49] = &gb.ppu
	gb.bus.io[0xFF4A] = &gb.ppu
	gb.bus.io[0xFF4B] = &gb.ppu

	// High RAM
	for i := 0xFF80; i < 0xFFFF; i++ {
		gb.bus.io[i] = &gb.cpu
	}

	// Interrupt Enable Register
	gb.bus.io[0xFFFF] = &gb.cpu

	if useBootrom {
		// Setup boot ROM
		for i := 0; i < len(dmgBootROM); i++ {
			gb.bus.io[i] = dmgBootROM
		}
	} else {
		// Simulate boot ROM side-effects
		gb.cpu.b = 0x00
		gb.cpu.c = 0x13
		gb.cpu.d = 0x00
		gb.cpu.e = 0xd8
		gb.cpu.h = 0x01
		gb.cpu.l = 0x4d
		gb.cpu.a = 0x01
		gb.cpu.f = 0xb0
		gb.cpu.sp = 0xfffe
		gb.cpu.pc = 0x0100
	}

	return gb
}

func (gb *Machine) lockBootROM() {
	// This remaps the cart to the bus, for the first 0x100 bytes.
	for i := 0; i < len(dmgBootROM); i++ {
		gb.bus.io[i] = gb.cart
	}
}

// UpdatePad updates the state of the gamepad.
func (gb *Machine) UpdatePad(pad Gamepad) {
	gb.cpu.gamepad = pad
}

// SetTrace enables or disables instruction tracing.
func (gb *Machine) SetTrace(trace bool) {
	gb.cpu.trace = trace
}

// GetFrameBuffer grabs the PPU framebuffer.
func (gb *Machine) GetFrameBuffer() *[160 * 144]uint32 {
	return &gb.ppu.screen
}

// Read reads a byte from memory.
func (gb *Machine) Read(addr uint16) uint8 {
	return gb.bus.Read(addr)
}

// Write writes a byte to memory.
func (gb *Machine) Write(addr uint16, value uint8) {
	if addr == 0xff50 {
		gb.lockBootROM()
	}

	gb.bus.Write(addr, value)
}

// Step increments the machine at the most atomic level.
func (gb *Machine) Step() {
	gb.stepInstruction()
}

// StepUntilStop runs the CPU until STOP.
func (gb *Machine) StepUntilStop() {
	for !gb.cpu.stop {
		gb.stepInstruction()
	}
}

// StepFrame steps until next vblank.
func (gb *Machine) StepFrame() uint {
	startClock := gb.cpu.clock
	for gb.ppu.clock >= 65664 {
		gb.Step()
	}
	for gb.ppu.clock < 65664 {
		gb.Step()
	}
	return gb.cpu.clock - startClock
}

// stepCycle forwards the state of the Gameboy while the CPU is running.
func (gb *Machine) stepCycle() {
	for i := 0; i < 4; i++ {
		gb.stepDMA()
		gb.stepPixel()
		gb.stepAudio()
		gb.checkTimers()
		gb.cpu.clock++
	}
}
