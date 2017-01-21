package gameboy

// This file implements the GameBoy memory bus.

// IO represents types that can handle bus I/O.
type IO interface {
	Read(addr uint16) uint8
	Write(addr uint16, value uint8)
}

// Bus implements the GameBoy memory bus.
type Bus struct {
	io [0x10000]IO
}

func (b *Bus) Read(addr uint16) uint8 {
	io := b.io[addr]

	if io == nil {
		return 0xFF
	}

	return io.Read(addr)
}

func (b *Bus) Write(addr uint16, value uint8) {
	io := b.io[addr]

	if io == nil {
		return
	}

	io.Write(addr, value)
}
