package gameboy

// ============================================================================
// Helper functions
func (cpu *CPU) af() uint16         { return uint16(cpu.a)<<8 | uint16(cpu.f) }
func (cpu *CPU) bc() uint16         { return uint16(cpu.b)<<8 | uint16(cpu.c) }
func (cpu *CPU) de() uint16         { return uint16(cpu.d)<<8 | uint16(cpu.e) }
func (cpu *CPU) hl() uint16         { return uint16(cpu.h)<<8 | uint16(cpu.l) }
func (cpu *CPU) setAF(value uint16) { cpu.a, cpu.f = uint8(value>>8), uint8(value&0xf0) }
func (cpu *CPU) setBC(value uint16) { cpu.b, cpu.c = uint8(value>>8), uint8(value&0xff) }
func (cpu *CPU) setDE(value uint16) { cpu.d, cpu.e = uint8(value>>8), uint8(value&0xff) }
func (cpu *CPU) setHL(value uint16) { cpu.h, cpu.l = uint8(value>>8), uint8(value&0xff) }

func (cpu *CPU) cf() bool { return cpu.f&carryFlag == carryFlag }
func (cpu *CPU) hf() bool { return cpu.f&halfCarryFlag == halfCarryFlag }
func (cpu *CPU) sf() bool { return cpu.f&subtractFlag == subtractFlag }
func (cpu *CPU) zf() bool { return cpu.f&zeroFlag == zeroFlag }

func (cpu *CPU) setCarryFlag(set bool)     { setBit(&cpu.f, 4, set) }
func (cpu *CPU) setHalfCarryFlag(set bool) { setBit(&cpu.f, 5, set) }
func (cpu *CPU) setSubtractFlag(set bool)  { setBit(&cpu.f, 6, set) }
func (cpu *CPU) setZeroFlag(val uint8)     { setBit(&cpu.f, 7, val == 0) }
func (cpu *CPU) clearFlags(flags uint8)    { cpu.f &= ^flags }

func (gb *Machine) fetchAt(reg uint16) uint8 {
	value := gb.Read(reg)
	gb.stepCycle()
	return value
}

func (gb *Machine) writeAt(reg uint16, value uint8) {
	gb.Write(reg, value)
	gb.stepCycle()
}

// ============================================================================
// CPU control ops
func (gb *Machine) cpuOpNop() {
	// Do nothing.
}

func (gb *Machine) cpuOpStop() {
	gb.cpu.stop = true
	//panic("stop not implemented")
}

func (gb *Machine) cpuOpHalt() {
	// TODO(john): This should only happen with DMG/SGB.
	// Skip next instruction (glitch.)
	gb.cpu.pc++

	// Do not halt if there are no interrupts enabled.
	if gb.cpu.ie&0x1f == 0 {
		return
	}

	gb.cpu.halt = true
}

func (gb *Machine) cpuOpSetCarryFlag() {
	gb.cpu.clearFlags(subtractFlag | halfCarryFlag)
	gb.cpu.setCarryFlag(true)
}

func (gb *Machine) cpuOpComplementCarryFlag() {
	gb.cpu.clearFlags(subtractFlag | halfCarryFlag)
	gb.cpu.setCarryFlag(!gb.cpu.cf())
}

func (gb *Machine) cpuOpRestart(vector uint8) {
	gb.cpuOpCall(uint16(vector))
}

func (gb *Machine) cpuOpReturnInterrupt() {
	gb.cpuOpReturn()
	gb.cpu.ime = true
}

func (gb *Machine) cpuOpUndefined() {
	panic("undefined opcode")
}

// ============================================================================
// CPU load ops
func (gb *Machine) cpuOpLoad(reg *uint8, value uint8) {
	*reg = value
}

func (gb *Machine) cpuOpLoadAt(reg uint16, value uint8) {
	gb.writeAt(reg, value)
}

func (gb *Machine) cpuOpLoadAt16(reg uint16, value uint16) {
	gb.cpuOpLoadAt(reg+0, uint8(value>>0))
	gb.cpuOpLoadAt(reg+1, uint8(value>>8))
}

func (gb *Machine) cpuOpLoadRR(r1 *uint8, r2 *uint8, value uint16) {
	*r1 = uint8(value >> 8)
	*r2 = uint8(value >> 0)
}

func (gb *Machine) cpuOpLoad16(reg *uint16, value uint16) {
	*reg = value
}

func (gb *Machine) cpuOpPush(dword uint16) {
	gb.stepCycle()
	gb.cpuPush(dword)
}

// ============================================================================
// CPU alu ops
func (gb *Machine) cpuOpIncrement(reg *uint8) {
	*reg++

	gb.cpu.clearFlags(zeroFlag | subtractFlag | halfCarryFlag)
	gb.cpu.setZeroFlag(*reg)
	gb.cpu.setHalfCarryFlag(*reg&0xf == 0)
}

func (gb *Machine) cpuOpIncrementAt(reg uint16) {
	value := gb.fetchAt(reg)
	gb.cpuOpIncrement(&value)
	gb.writeAt(reg, value)
}

func (gb *Machine) cpuOpIncrementRR(r1 *uint8, r2 *uint8) {
	*r2++
	if *r2 == 0x00 {
		*r1++
	}
	gb.stepCycle()
}

func (gb *Machine) cpuOpIncrement16(reg *uint16) {
	*reg++
	gb.stepCycle()
}

func (gb *Machine) cpuOpDecrement(reg *uint8) {
	*reg--

	gb.cpu.clearFlags(zeroFlag | halfCarryFlag)
	gb.cpu.setSubtractFlag(true)
	gb.cpu.setZeroFlag(*reg)
	gb.cpu.setHalfCarryFlag(*reg&0xf == 0xf)
}

func (gb *Machine) cpuOpDecrementAt(reg uint16) {
	value := gb.fetchAt(reg)
	gb.cpuOpDecrement(&value)
	gb.writeAt(reg, value)
}

func (gb *Machine) cpuOpDecrementRR(r1 *uint8, r2 *uint8) {
	*r2--
	if *r2 == 0xff {
		*r1--
	}
	gb.stepCycle()
}

func (gb *Machine) cpuOpDecrement16(reg *uint16) {
	*reg--
	gb.stepCycle()
}

func (gb *Machine) cpuOpRotateLeft(reg *uint8) {
	// Calculate result of rotate left
	value := uint(*reg) << 1
	if gb.cpu.cf() {
		value |= 0x01
	}

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(7) != 0)

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpRotateRight(reg *uint8) {
	// Calculate result of rotate right
	value := uint(*reg) >> 1
	if gb.cpu.cf() {
		value |= 0x80
	}

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(0) != 0)

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpRotateLeftCarry(reg *uint8) {
	// Calculate result of rotate left with carry
	value := uint(*reg)
	value = value<<1 | value>>7

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(7) != 0)

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpRotateRightCarry(reg *uint8) {
	// Calculate result of rotate right with carry
	value := uint(*reg)
	value = value>>1 | value<<7

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(0) != 0)

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpDecimalAdjust(reg *uint8) {
	// Attempts to get this right: 6
	// Wow. This is a challenging one. VERY confusing instruction.

	adj := uint8(0)
	if gb.cpu.sf() {
		if gb.cpu.hf() {
			adj |= 0x06
		}
		if gb.cpu.cf() {
			adj |= 0x60
		}
		*reg -= adj
	} else {
		if gb.cpu.hf() || (*reg&0xf) > 0x9 {
			adj |= 0x06
		}
		if gb.cpu.cf() || *reg > 0x99 {
			adj |= 0x60
		}
		*reg += adj
	}

	gb.cpu.clearFlags(halfCarryFlag)
	gb.cpu.setZeroFlag(*reg)
	gb.cpu.setCarryFlag(adj >= 0x60)
}

func (gb *Machine) cpuOpBitwiseComplement(reg *uint8) uint {
	*reg ^= 0xff

	gb.cpu.setSubtractFlag(true)
	gb.cpu.setHalfCarryFlag(true)

	return 0
}

func (gb *Machine) cpuOpAddRR(h *uint8, l *uint8, value uint16) {
	gb.stepCycle()

	hl := uint(*h)<<8 + uint(*l)

	rb := hl + uint(value)
	rn := (hl & 0xfff) + uint(value&0xfff)

	*h = uint8(rb >> 8)
	*l = uint8(rb & 0xff)

	gb.cpu.setSubtractFlag(false)
	gb.cpu.setHalfCarryFlag(rn > 0x0fff)
	gb.cpu.setCarryFlag(rb > 0xffff)
}

func (gb *Machine) cpuOpAdd(reg *uint8, value uint8, carry bool) {
	c := uint8(0)
	if carry && gb.cpu.cf() {
		c = 1
	}

	// Calculate result
	rn := uint(*reg) + uint(value) + uint(c)
	rh := *reg&0xf + value&0xf + c
	*reg = uint8(rn)

	// Update flags
	gb.cpu.clearFlags(subtractFlag)
	gb.cpu.setZeroFlag(*reg)
	gb.cpu.setHalfCarryFlag(rh > 0x0f)
	gb.cpu.setCarryFlag(rn > 0xff)
}

func (gb *Machine) cpuOpAddSP(value int8) {
	sp := int(gb.cpu.sp)
	rn := sp + int(value)
	rh := sp&0xfff + int(value)
	gb.cpu.sp = uint16(rn)

	gb.cpu.clearFlags(allFlags)
	gb.cpu.setHalfCarryFlag(rh > 0x0fff)
	gb.cpu.setCarryFlag(rn > 0xffff)
}

func (gb *Machine) cpuOpSub(reg *uint8, value uint8, carry bool) {
	c := uint8(0)
	if carry && gb.cpu.cf() {
		c = 1
	}

	rn := uint(*reg) - uint(value) - uint(c)
	rh := *reg&0xf - value&0xf - c
	*reg = uint8(rn)

	// Update flags
	gb.cpu.setZeroFlag(*reg)
	gb.cpu.setSubtractFlag(true)
	gb.cpu.setHalfCarryFlag(rh > 0x0f)
	gb.cpu.setCarryFlag(rn > 0xff)
}

func (gb *Machine) cpuOpAnd(reg *uint8, value uint8) {
	*reg &= value

	gb.cpu.clearFlags(subtractFlag | carryFlag)
	gb.cpu.setZeroFlag(*reg)
	gb.cpu.setHalfCarryFlag(true)
}

func (gb *Machine) cpuOpXor(reg *uint8, value uint8) {
	*reg ^= value

	gb.cpu.clearFlags(subtractFlag | carryFlag | halfCarryFlag)
	gb.cpu.setZeroFlag(*reg)
}

func (gb *Machine) cpuOpOr(reg *uint8, value uint8) {
	*reg |= value

	gb.cpu.clearFlags(subtractFlag | carryFlag | halfCarryFlag)
	gb.cpu.setZeroFlag(*reg)
}

func (gb *Machine) cpuOpCompare(reg *uint8, value uint8) {
	res := *reg
	gb.cpuOpSub(&res, value, false)
}

// ============================================================================
// CPU jump ops
func (gb *Machine) cpuOpJump(addr uint16) {
	gb.cpu.pc = addr
	gb.stepCycle()
}

func (gb *Machine) cpuOpJumpFlag(flag bool, addr uint16) {
	if flag {
		gb.cpuOpJump(addr)
	}
}

func (gb *Machine) cpuOpJumpRel(value int) {
	gb.cpuOpJump(uint16(int(gb.cpu.pc) + value))
}

func (gb *Machine) cpuOpJumpRelFlag(flag bool, value int) {
	if flag {
		gb.cpuOpJump(uint16(int(gb.cpu.pc) + value))
	}
}

func (gb *Machine) cpuOpCall(addr uint16) {
	gb.cpuPush(gb.cpu.pc)
	gb.cpuOpJump(addr)
}

func (gb *Machine) cpuOpCallFlag(flag bool, addr uint16) {
	if flag {
		gb.cpuPush(gb.cpu.pc)
		gb.cpuOpJump(addr)
	}
}

func (gb *Machine) cpuOpReturn() {
	gb.cpuOpJump(gb.cpuPop())
}

func (gb *Machine) cpuOpReturnFlag(flag bool) {
	gb.stepCycle()
	if flag {
		gb.cpuOpJump(gb.cpuPop())
	}
}

// ============================================================================
// CPU bit ops (CB prefix)
// N.B.: There is some overlap with the above; CB-prefixed instructions behave
//       differently, however. i.e. RLCA does NOT set the ZF.
func (gb *Machine) cpuOpCBRotateLeftCarry(reg *uint8) {
	// Calculate result of rotate left with carry
	value := uint(*reg)
	value = value<<1 | value>>7

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(7) != 0)
	gb.cpu.setZeroFlag(uint8(value))

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpCBRotateRightCarry(reg *uint8) {
	// Calculate result of rotate right with carry
	value := uint(*reg)
	value = value>>1 | value<<7

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(0) != 0)
	gb.cpu.setZeroFlag(uint8(value))

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpCBRotateLeft(reg *uint8) {
	// Calculate result of rotate left
	value := uint(*reg) << 1
	if gb.cpu.cf() {
		value |= 0x01
	}

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(7) != 0)
	gb.cpu.setZeroFlag(uint8(value))

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpCBRotateRight(reg *uint8) {
	// Calculate result of rotate right
	value := uint(*reg) >> 1
	if gb.cpu.cf() {
		value |= 0x80
	}

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(0) != 0)
	gb.cpu.setZeroFlag(uint8(value))

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpCBArithmeticShiftLeft(reg *uint8) {
	// Calculate result of shift left
	value := uint(*reg) << 1

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(7) != 0)
	gb.cpu.setZeroFlag(uint8(value))

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpCBArithmeticShiftRight(reg *uint8) {
	// Calculate result of shift right
	value := (*reg >> 1) | (*reg & 0x80)

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(0) != 0)
	gb.cpu.setZeroFlag(uint8(value))

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpCBSwap(reg *uint8) {
	// Calculate result of swap
	value := *reg<<4 | *reg>>4

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setZeroFlag(uint8(value))

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpCBLogicalShiftRight(reg *uint8) {
	// Calculate result of shift right
	value := uint(*reg) >> 1

	// Update flags
	gb.cpu.clearFlags(allFlags)
	gb.cpu.setCarryFlag(*reg&bit(0) != 0)
	gb.cpu.setZeroFlag(uint8(value))

	// Set register
	*reg = uint8(value)
}

func (gb *Machine) cpuOpCBBit(b uint8, reg *uint8) {
	// Set flags based on bit
	gb.cpu.clearFlags(zeroFlag | subtractFlag | halfCarryFlag)
	gb.cpu.setZeroFlag(*reg & bit(b))
	gb.cpu.setHalfCarryFlag(true)
}

func (gb *Machine) cpuOpCBBitReset(bit uint8, reg *uint8) {
	setBit(reg, bit, false)
}

func (gb *Machine) cpuOpCBBitSet(bit uint8, reg *uint8) {
	setBit(reg, bit, true)
}
