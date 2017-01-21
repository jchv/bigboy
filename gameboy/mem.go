package gameboy

var (
	romSize = map[uint8]uint{
		0x00: 0x8000 << 0,
		0x01: 0x8000 << 1,
		0x02: 0x8000 << 2,
		0x03: 0x8000 << 3,
		0x04: 0x8000 << 4,
		0x05: 0x8000 << 5,
		0x06: 0x8000 << 6,
		0x07: 0x8000 << 7,
		0x52: 0x8000 * 36,
		0x53: 0x8000 * 40,
		0x54: 0x8000 * 48,
	}

	ramSize = map[uint8]uint{
		0x00: 0x0000,
		0x01: 0x0800,
		0x02: 0x2000,
		0x03: 0x8000,
	}
)

// WRAM represents the work RAM.
type WRAM [0x2000]byte

// Read reads a byte from memory.
func (ram *WRAM) Read(addr uint16) uint8 {
	return ram[addr&0x1fff]
}

// Write writes a byte to memory.
func (ram *WRAM) Write(addr uint16, value uint8) {
	ram[addr&0x1fff] = value
}

// ROM represents a cartridge without a MBC chip.
type ROM []byte

// Read reads a byte from memory.
func (rom ROM) Read(addr uint16) uint8 {
	if int(addr) >= len(rom) {
		return 0xff
	}
	return rom[addr]
}

// Write writes a byte to memory.
func (rom ROM) Write(addr uint16, value uint8) {
	return
}

// MBC1Cartridge implements a cartridge containing the MBC1 mapper.
type MBC1Cartridge struct {
	rom []byte
	ram []byte

	enableram bool

	rombank uint
	rambank uint
}

// NewMBC1Cartridge creates a new MBC1Cartridge with the given ROM.
func NewMBC1Cartridge(rom []byte) *MBC1Cartridge {
	return &MBC1Cartridge{
		rom:       rom,
		ram:       make([]byte, 0x2000),
		enableram: false,
		rombank:   0,
		rambank:   0,
	}
}

// Read reads a byte from memory.
func (cart *MBC1Cartridge) Read(addr uint16) uint8 {
	switch {
	case addr >= 0x0000 && addr < 0x4000:
		if int(addr) >= len(cart.rom) {
			break
		}

		return cart.rom[addr]

	case addr >= 0x4000 && addr < 0x8000:
		romaddr := uint(addr & 0x3fff)

		bank := cart.rombank
		if bank&0x1f == 0 {
			bank++
		}

		romaddr += bank << 14
		if int(romaddr) >= len(cart.rom) {
			break
		}

		return cart.rom[romaddr]

	case addr >= 0xa000 && addr < 0xc000:
		ramaddr := uint(addr & 0x1fff)

		bank := cart.rambank
		ramaddr += bank << 13
		if int(addr) >= len(cart.ram) {
			break
		}

		return cart.ram[ramaddr]
	}

	return 0xff
}

// Write writes a byte to memory.
func (cart *MBC1Cartridge) Write(addr uint16, value uint8) {
	switch {
	case addr >= 0x0000 && addr < 0x2000:
		cart.enableram = value&0xf == 0xa
	case addr >= 0x2000 && addr < 0x4000:
		cart.rombank = uint(value)
	}
}
