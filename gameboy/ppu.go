package gameboy

import "sort"

var (
	rgbColors = [4]uint32{0xFFD7E894, 0xFFAEC440, 0xFF527F39, 0xFF204631}
)

// Object contains the state of an object.
type Object struct {
	x, y, tile, attr, data uint
}

type Objects [10]Object

func (s Objects) Len() int {
	return len(s)
}

func (s Objects) Less(i, j int) bool {
	return s[i].x < s[j].x
}

func (s Objects) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// PPU implements the Gameboy display controller.
type PPU struct {
	vram [0x2000]uint8
	oam  [160]uint8
	bgp  [4]uint8
	obp  [2][4]uint8
	bgpd [64]uint8
	obpd [64]uint8

	bgColor, bgPalette, bgPriority uint16
	fgColor, fgPalette, fgPriority uint16

	clock      int
	lx         uint
	screen     [160 * 144]uint32
	objects    Objects
	numObjects uint

	// LCD Control Register (0xFF40)
	lcdDisplayEnable    bool // 0xFF40 << 7
	windowTilemapEnable bool // 0xFF40 << 6
	windowDisplayEnable bool // 0xFF40 << 5
	bgTileDataSelect    bool // 0xFF40 << 4
	bgTileMapSelect     bool // 0xFF40 << 3
	objSize             bool // 0xFF40 << 2
	objDisplay          bool // 0xFF40 << 1
	bgDisplay           bool // 0xFF40 << 0

	// LCD Status Register
	lycInterrupt    bool // 0xFF41 << 6
	oamInterrupt    bool // 0xFF41 << 5
	vblankInterrupt bool // 0xFF41 << 4
	hblankInterrupt bool // 0xFF41 << 3
	modeHi          bool // 0xFF41 << 1
	modeLo          bool // 0xFF41 << 0

	// LCD Positioning and Scrolling
	scrollY uint8 // 0xFF42
	scrollX uint8 // 0xFF43
	ly      uint8 // 0xFF44
	lyComp  uint8 // 0xFF45

	// LCD OAM DMA Transfers
	dmaAddr   uint8 // 0xFF46
	dmaEnable bool
	dmaClock  uint

	// Window area
	winYPos uint8 // 0xFF4A
	winXPos uint8 // 0xFF4B

	// background
	backgroundData uint
	backgroundAttr uint
	windowData     uint
	windowAttr     uint
}

func (ppu *PPU) Reset() {
	*ppu = PPU{}

	ppu.obp[0] = [4]uint8{3, 3, 3, 3}
	ppu.obp[1] = [4]uint8{3, 3, 3, 3}
}

func (ppu *PPU) Read(addr uint16) uint8 {
	switch {
	case addr >= 0x8000 && addr < 0xA000:
		return ppu.vram[addr&0x1fff]
	case addr >= 0xFE00 && addr < 0xFEA0:
		return ppu.oam[addr-0xFE00]
	case addr == 0xFF40:
		return ppu.lcdControlReg()
	case addr == 0xFF41:
		return ppu.lcdStatusReg()
	case addr == 0xFF42:
		return ppu.scrollY
	case addr == 0xFF43:
		return ppu.scrollX
	case addr == 0xFF44:
		return ppu.ly
	case addr == 0xFF45:
		return ppu.lyComp
	case addr == 0xFF47:
		var val uint8
		val |= (ppu.bgp[0] & 3) << 0
		val |= (ppu.bgp[1] & 3) << 2
		val |= (ppu.bgp[2] & 3) << 4
		val |= (ppu.bgp[3] & 3) << 6
		return val
	case addr == 0xFF48:
		var val uint8
		val |= (ppu.obp[0][0] & 3) << 0
		val |= (ppu.obp[0][1] & 3) << 2
		val |= (ppu.obp[0][2] & 3) << 4
		val |= (ppu.obp[0][3] & 3) << 6
		return val
	case addr == 0xFF49:
		var val uint8
		val |= (ppu.obp[1][0] & 3) << 0
		val |= (ppu.obp[1][1] & 3) << 2
		val |= (ppu.obp[1][2] & 3) << 4
		val |= (ppu.obp[1][3] & 3) << 6
		return val
	case addr == 0xFF4A:
		return ppu.winYPos
	case addr == 0xFF4B:
		return ppu.winXPos
	}

	return 0xFF
}

func (ppu *PPU) Write(addr uint16, value uint8) {
	switch {
	case addr >= 0x8000 && addr < 0xA000:
		ppu.vram[addr&0x1fff] = value
	case addr >= 0xFE00 && addr < 0xFEA0:
		ppu.oam[addr-0xFE00] = value
	case addr == 0xFF40:
		ppu.setLCDControlReg(value)
	case addr == 0xFF41:
		ppu.setLCDStatusReg(value)
	case addr == 0xFF42:
		ppu.scrollY = value
	case addr == 0xFF43:
		ppu.scrollX = value
	case addr == 0xFF44:
		ppu.ly = 0
		ppu.lx = 0
		ppu.clock = 0
	case addr == 0xFF45:
		ppu.lyComp = value
	case addr == 0xFF47:
		ppu.bgp[0] = (value >> 0) & 3
		ppu.bgp[1] = (value >> 2) & 3
		ppu.bgp[2] = (value >> 4) & 3
		ppu.bgp[3] = (value >> 6) & 3
	case addr == 0xFF48:
		ppu.obp[0][0] = (value >> 0) & 3
		ppu.obp[0][1] = (value >> 2) & 3
		ppu.obp[0][2] = (value >> 4) & 3
		ppu.obp[0][3] = (value >> 6) & 3
	case addr == 0xFF49:
		ppu.obp[1][0] = (value >> 0) & 3
		ppu.obp[1][1] = (value >> 2) & 3
		ppu.obp[1][2] = (value >> 4) & 3
		ppu.obp[1][3] = (value >> 6) & 3
	case addr == 0xFF4A:
		ppu.winYPos = value
	case addr == 0xFF4B:
		ppu.winXPos = value
	}
}

func (ppu *PPU) lcdControlReg() uint8 {
	value := uint8(0)
	setBit(&value, 7, ppu.lcdDisplayEnable)
	setBit(&value, 6, ppu.windowTilemapEnable)
	setBit(&value, 5, ppu.windowDisplayEnable)
	setBit(&value, 4, ppu.bgTileDataSelect)
	setBit(&value, 3, ppu.bgTileMapSelect)
	setBit(&value, 2, ppu.objSize)
	setBit(&value, 1, ppu.objDisplay)
	setBit(&value, 0, ppu.bgDisplay)
	return value
}

func (ppu *PPU) setLCDControlReg(value uint8) {
	getBit(value, 7, &ppu.lcdDisplayEnable)
	getBit(value, 6, &ppu.windowTilemapEnable)
	getBit(value, 5, &ppu.windowDisplayEnable)
	getBit(value, 4, &ppu.bgTileDataSelect)
	getBit(value, 3, &ppu.bgTileMapSelect)
	getBit(value, 2, &ppu.objSize)
	getBit(value, 1, &ppu.objDisplay)
	getBit(value, 0, &ppu.bgDisplay)
}

func (ppu *PPU) lcdStatusReg() uint8 {
	result := uint8(0)
	if ppu.lycInterrupt {
		result |= 1 << 6
	}
	if ppu.oamInterrupt {
		result |= 1 << 5
	}
	if ppu.vblankInterrupt {
		result |= 1 << 4
	}
	if ppu.hblankInterrupt {
		result |= 1 << 3
	}
	if ppu.modeHi {
		result |= 1 << 1
	}
	if ppu.modeLo {
		result |= 1 << 0
	}
	return result
}

func (ppu *PPU) setLCDStatusReg(v uint8) {
	ppu.lycInterrupt = v&(1<<6) != 0
	ppu.oamInterrupt = v&(1<<5) != 0
	ppu.vblankInterrupt = v&(1<<4) != 0
	ppu.hblankInterrupt = v&(1<<3) != 0
	ppu.modeHi = v&(1<<1) != 0
	ppu.modeLo = v&(1<<0) != 0
}

func (ppu *PPU) initScanline() {
	var objHeight uint

	objHeight = 8
	if ppu.objSize {
		objHeight = 16
	}

	ppu.numObjects = 0

	for n := 0; n < 40; n++ {
		s := &ppu.objects[ppu.numObjects]
		s.y = uint(ppu.ly) - (uint(ppu.oam[n*4+0]) - 16)
		s.x = uint(ppu.oam[n*4+1]) - 8
		s.tile = uint(ppu.oam[n*4+2])
		s.attr = uint(ppu.oam[n*4+3])

		if s.y >= objHeight {
			continue
		}

		if s.attr&0x40 != 0 {
			s.y ^= (objHeight - 1)
		}

		tileDataAddr := (s.tile << 4) + (s.y << 1)
		s.data = uint(ppu.vram[tileDataAddr+0]) << 0
		s.data |= uint(ppu.vram[tileDataAddr+1]) << 8

		if s.attr&0x20 != 0 {
			// Bit twiddling hack.
			s.data = ((s.data >> 1) & 0x5555) | ((s.data & 0x5555) << 1)
			s.data = ((s.data >> 2) & 0x3333) | ((s.data & 0x3333) << 2)
			s.data = ((s.data >> 4) & 0x0F0F) | ((s.data & 0x0F0F) << 4)
			s.data = ((s.data >> 8) & 0x00FF) | ((s.data & 0x00FF) << 8)
		}

		ppu.numObjects++
		if ppu.numObjects == 10 {
			break
		}
	}

	sort.Stable(ppu.objects)
}

func (ppu *PPU) readTileLine(sel bool, x, y uint) uint {
	var tileMapBase, tileMapAddr, tileDataAddr uint

	// Determine tilemap base
	if sel {
		tileMapBase = 0x1C00
	} else {
		tileMapBase = 0x1800
	}

	// Determine address of tile in tilemap
	tileX, tileY := x/8, y/8
	tileMapAddr = tileMapBase + ((tileY<<5)+tileX)&0x03FF

	// Determine address of tile data
	if ppu.bgTileDataSelect {
		tileDataAddr = 0x0000 + uint(ppu.vram[tileMapAddr])<<4 + ((y & 0x7) << 1)
	} else {
		tileDataAddr = uint(0x1000+int(int8(ppu.vram[tileMapAddr]))<<4) + ((y & 0x7) << 1)
	}

	// Get data from tile data address
	data := uint(ppu.vram[tileDataAddr+0]) << 0
	data |= uint(ppu.vram[tileDataAddr+1]) << 8

	return data
}

func (ppu *PPU) pixel() {
	ppu.bgColor = 0
	ppu.bgPalette = 0

	ppu.fgColor = 0
	ppu.fgPalette = 0

	color := uint16(0)

	// TODO: implement pixel fifo register

	// this isn't much different than the current code, except instead of
	// reading the line all at once, it is read one pixel at a time and
	// stored in a register. the side-effect is that we can count cycles
	// more accurately.

	if ppu.bgDisplay {
		scrolly := uint(ppu.ly+ppu.scrollY) & 0xFF
		scrollx := uint(uint(ppu.scrollX)+ppu.lx) & 0xFF
		scrollBit := scrollx & 0x7

		if scrollBit == 0 || ppu.lx == 0 {
			ppu.backgroundData = ppu.readTileLine(ppu.bgTileMapSelect, scrollx, scrolly)
		}

		index := uint(0)
		if ppu.backgroundData&(0x0080>>scrollBit) != 0 {
			index |= 1
		}
		if ppu.backgroundData&(0x8000>>scrollBit) != 0 {
			index |= 2
		}

		ppu.bgColor = uint16(ppu.bgp[index])
		ppu.bgPalette = uint16(index)
	}

	if ppu.windowDisplayEnable {
		scrolly := uint(ppu.ly) - uint(ppu.winYPos)
		scrollx := ppu.lx + 7 - uint(ppu.winXPos)
		scrollBit := scrollx & 0x7

		if scrolly < 144 && scrollx < 160 {
			if scrollBit == 0 || ppu.lx == 0 {
				ppu.windowData = ppu.readTileLine(ppu.windowTilemapEnable, scrollx, scrolly)
			}

			index := uint(0)
			if ppu.windowData&(0x0080>>scrollBit) != 0 {
				index |= 1
			}
			if ppu.windowData&(0x8000>>scrollBit) != 0 {
				index |= 2
			}

			ppu.bgColor = uint16(ppu.bgp[index])
			ppu.bgPalette = uint16(index)
		}
	}

	if ppu.objDisplay {
		for n := int(ppu.numObjects) - 1; n >= 0; n-- {
			s := &ppu.objects[n]
			scrollBit := ppu.lx - s.x

			if scrollBit > 7 {
				continue
			}

			index := uint(0)
			if s.data&(0x0080>>scrollBit) != 0 {
				index |= 1
			}
			if s.data&(0x8000>>scrollBit) != 0 {
				index |= 2
			}
			if index == 0 {
				continue
			}

			if s.attr&0x10 != 0 {
				ppu.fgColor = uint16(ppu.obp[1][index])
			} else {
				ppu.fgColor = uint16(ppu.obp[0][index])
			}
			ppu.fgPalette = uint16(index)
		}
	}

	// Implement priority/transparency
	if ppu.fgPalette == 0 {
		color = ppu.bgColor
	} else if ppu.bgPalette == 0 {
		color = ppu.fgColor
	} else if ppu.fgPriority != 0 {
		color = ppu.fgColor
	} else {
		color = ppu.bgColor
	}

	ppu.screen[uint(ppu.ly)*160+ppu.lx] = rgbColors[color]
}

func (gb *Machine) stepPixel() {
	ppu := &gb.ppu

	// TODO: implement variable hblank timings
	// This timing code looks nice but it is a lie.
	hclock := ppu.clock % 456
	switch {
	case ppu.clock < 65664:
		switch {
		case hclock == 0:
			ppu.modeHi, ppu.modeLo = true, false

			ppu.lx = 0

			if ppu.lcdDisplayEnable {
				ppu.initScanline()
			}

			gb.Interrupt(intLCDStat)

		case hclock >= 80 && hclock < 80+160:
			ppu.modeHi, ppu.modeLo = true, true

			if ppu.lcdDisplayEnable {
				ppu.pixel()
			}

			ppu.lx++

		case hclock == 80+160:
			ppu.modeHi, ppu.modeLo = false, false
			// TODO(john): DMA should be handled here

		case hclock == 455:
			ppu.ly++
		}
		break
	case ppu.clock == 65664:
		ppu.modeHi, ppu.modeLo = false, true

		// Entering VBlank period.
		if ppu.lcdDisplayEnable {
			gb.Interrupt(intVBlank)
		}
	case ppu.clock < 70223:
		switch {
		case hclock == 455:
			ppu.ly++
		}
	case ppu.clock == 70223:
		// Screen refresh next cycle
		ppu.clock = -1
		ppu.ly = 0
	}

	ppu.clock++
}
