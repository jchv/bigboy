package gameboy

import "fmt"

// Interrupt bits (for irq/ie)
const (
	intVBlank  = 1 << 0
	intLCDStat = 1 << 1
	intTimer   = 1 << 2
	intSerial  = 1 << 3
	intGamepad = 1 << 4
)

// Flags
const (
	carryFlag     = 1 << 4
	halfCarryFlag = 1 << 5
	subtractFlag  = 1 << 6
	zeroFlag      = 1 << 7
	allFlags      = carryFlag | halfCarryFlag | subtractFlag | zeroFlag
)

var (
	interruptVectorMap = map[uint8]uint16{
		intVBlank:  0x0040,
		intLCDStat: 0x0048,
		intTimer:   0x0050,
		intSerial:  0x0058,
		intGamepad: 0x0060,
	}
)

// CPU implements the GameBoy DMG processor.
type CPU struct {
	// Registers
	a, f uint8
	b, c uint8
	d, e uint8
	h, l uint8

	sp uint16
	pc uint16

	// High RAM
	hram [127]byte

	// Interrupts
	irq uint8
	ie  uint8
	ime bool

	// Processor state
	clock uint
	halt  bool
	stop  bool

	// Gamepad state
	gamepad Gamepad
	button  bool
	dpad    bool

	// DMA state
	dma      bool
	dmabank  uint8
	dmaindex uint16

	// Timer state
	timer, tima, tma uint8
	div              uint

	// Debug state
	trace bool
}

func (cpu *CPU) Read(addr uint16) uint8 {
	switch {
	case addr == 0xFF00:
		value := uint8(0xF)

		// Button bits
		button := uint8(0xF)
		setBit(&button, 0, !cpu.gamepad.A)
		setBit(&button, 1, !cpu.gamepad.B)
		setBit(&button, 2, !cpu.gamepad.Select)
		setBit(&button, 3, !cpu.gamepad.Start)
		if cpu.button {
			value &= button
		}

		// DPad bits
		dpad := uint8(0xF)
		setBit(&dpad, 0, !cpu.gamepad.Right)
		setBit(&dpad, 1, !cpu.gamepad.Left)
		setBit(&dpad, 2, !cpu.gamepad.Up)
		setBit(&dpad, 3, !cpu.gamepad.Down)
		if cpu.dpad {
			value &= dpad
		}

		// Fire interrupt if anything pressed
		if value != 0xF {
			cpu.ie |= intGamepad
		}

		// Select bits
		setBit(&value, 4, !cpu.dpad)
		setBit(&value, 5, !cpu.button)

		return value
	case addr == 0xFF01 || addr == 0xFF02:
		// Serial bus not implemented
		return 0xFF
	case addr == 0xFF04:
		return uint8(cpu.div)
	case addr == 0xFF05:
		return cpu.tima
	case addr == 0xFF06:
		return cpu.tma
	case addr == 0xFF07:
		return cpu.timer
	case addr == 0xFF0F:
		return cpu.irq
	case addr >= 0xFF80 && addr < 0xFFFF:
		return cpu.hram[addr&0x7F]
	case addr == 0xFFFF:
		return cpu.ie & 0x1f
	}
	return 0xFF
}

func (cpu *CPU) Write(addr uint16, value uint8) {
	switch {
	case addr == 0xFF00:
		getBit(^value, 4, &cpu.dpad)
		getBit(^value, 5, &cpu.button)
	case addr == 0xFF04:
		cpu.div = 0
	case addr == 0xFF05:
		cpu.tima = value
	case addr == 0xFF06:
		cpu.tma = value
	case addr == 0xFF07:
		cpu.timer = value
	case addr == 0xFF0F:
		cpu.irq = value
	case addr == 0xFF46:
		cpu.dma = true
		cpu.dmabank = value
		cpu.dmaindex = 0
	case addr >= 0xFF80 && addr < 0xFFFF:
		cpu.hram[addr&0x7F] = value
	case addr == 0xFFFF:
		cpu.ie = value & 0x1f
	}
}

// cpuFetch fetches a byte from pc and increments pc
func (gb *Machine) cpuFetch() uint8 {
	val := gb.Read(gb.cpu.pc)
	gb.cpu.pc++
	gb.stepCycle()

	return val
}

// cpuFetchSigned fetches a signed byte from pc and increments pc
func (gb *Machine) cpuFetchSigned() int8 {
	val := int8(gb.Read(gb.cpu.pc))
	gb.cpu.pc++
	gb.stepCycle()

	return val
}

// cpuFetch16 fetches a dword from pc and increments pc
func (gb *Machine) cpuFetch16() uint16 {
	val := uint16(gb.Read(gb.cpu.pc)) << 0
	gb.cpu.pc++
	gb.stepCycle()

	val |= uint16(gb.Read(gb.cpu.pc)) << 8
	gb.cpu.pc++
	gb.stepCycle()

	return val
}

// cpuPush pushes a dword onto the stack.
func (gb *Machine) cpuPush(dword uint16) {
	gb.cpu.sp--
	gb.Write(gb.cpu.sp, uint8(dword>>8))
	gb.stepCycle()

	gb.cpu.sp--
	gb.Write(gb.cpu.sp, uint8(dword>>0))
	gb.stepCycle()
}

// cpuPop pops a dword off of the stack.
func (gb *Machine) cpuPop() uint16 {
	var value uint16

	value |= uint16(gb.Read(gb.cpu.sp)) << 0
	gb.cpu.sp++
	gb.stepCycle()

	value |= uint16(gb.Read(gb.cpu.sp)) << 8
	gb.cpu.sp++
	gb.stepCycle()

	return value
}

// Interrupt sets an interrupt request.
func (gb *Machine) Interrupt(i uint8) {
	if gb.cpu.ie&i != 0 {
		gb.cpu.halt = false
	}
	gb.cpu.irq |= i
}

// cpuInterrupt runs an interupt vector.
func (gb *Machine) cpuInterrupt(vector uint16) {
	// Set flags to stop interrupts.
	gb.cpu.ime = false

	// Push the program counter onto the stack.
	gb.cpuPush(gb.cpu.pc)

	// Set PC to vector.
	gb.cpu.pc = vector
}

func (gb *Machine) checkTimers() {
	// TODO(john): Implement proper timings and behavior.
	// See http://gbdev.gg8.se/wiki/articles/Timer_Obscure_Behaviour

	// Only update div every 256th clock.
	if gb.cpu.clock&0xff == 0 {
		gb.cpu.div++
	}

	// Figure out how often timer should fire.
	mask := uint(0)
	switch gb.cpu.timer & 0x7 {
	case 0x5:
		// 262144 Hz
		mask = 0x00f
	case 0x6:
		// 65536 Hz
		mask = 0x03f
	case 0x7:
		// 16384 Hz
		mask = 0x0ff
	case 0x4:
		// 4096 Hz
		mask = 0x3ff
	default:
		// Disabled
		return
	}

	// Increment tima.
	if gb.cpu.clock&mask == 0 {
		gb.cpu.tima++
		if gb.cpu.tima == 0 {
			gb.cpu.tima = gb.cpu.tma
			gb.Interrupt(intTimer)
		}
	}
}

func (gb *Machine) checkInterrupts() {
	if !gb.cpu.ime {
		return
	}

	for flag, vector := range interruptVectorMap {
		if gb.cpu.irq&gb.cpu.ie&flag != 0 {
			gb.cpu.halt = false
			gb.cpu.irq &= ^flag
			gb.cpuInterrupt(vector)
			return
		}
	}
}

func (gb *Machine) stepDMA() {
	if gb.cpu.dma {
		dstindex := 0xFE00 + gb.cpu.dmaindex
		srcindex := uint16(gb.cpu.dmabank)<<8 + gb.cpu.dmaindex
		src := gb.Read(srcindex)
		gb.Write(dstindex, src)
		//fmt.Printf("dma%02x: %04x = (%04x) %02x\n", gb.cpu.dmaindex, dstindex, srcindex, src)

		gb.cpu.dmaindex++
		if gb.cpu.dmaindex == 160 {
			gb.cpu.dma = false
		}
	}
}

func (gb *Machine) trace() {
	// Decode instruction
	ins := []byte{}
	rdr := busReader{bus: gb, addr: gb.cpu.pc}
	asm := Disassemble(&rdr)

	// Get instruction bytes
	for i := gb.cpu.pc; i < rdr.addr; i++ {
		ins = append(ins, gb.Read(i))
	}

	// Pad instruction bytes
	insstr := fmt.Sprintf("% 02x", ins)
	for len(insstr) < 13 {
		insstr += " "
	}

	// Pad asm str
	asmstr := fmt.Sprintf("%s", asm)
	for len(asmstr) < 16 {
		asmstr += " "
	}

	fmt.Printf("%s %s | b=%02x c=%02x d=%02x e=%02x h=%02x l=%02x a=%02x f=%04b sp=%04x pc=%04x clk=%d\n", insstr, asmstr, gb.cpu.b, gb.cpu.c, gb.cpu.d, gb.cpu.e, gb.cpu.h, gb.cpu.l, gb.cpu.a, gb.cpu.f>>4, gb.cpu.sp, gb.cpu.pc, gb.cpu.clock/4)
}

func (gb *Machine) stepInstruction() {
	if gb.cpu.halt {
		// Halted still
		gb.stepCycle()
		return
	}

	// Check interrupts
	gb.checkInterrupts()

	// Trace mode
	if gb.cpu.trace {
		gb.trace()
	}

	// Fetch next instruction.
	op := gb.cpuFetch()

	// Dispatch.
	if op == 0xcb {
		gb.cpuDispatchCB(gb.cpuFetch())
	} else {
		gb.cpuDispatch(op)
	}
}

func (gb *Machine) cpuDispatch(op uint8) {
	cpu := &gb.cpu

	switch op {
	case 0x00:
		gb.cpuOpNop()
	case 0x01:
		gb.cpuOpLoadRR(&cpu.b, &cpu.c, gb.cpuFetch16())
	case 0x02:
		gb.cpuOpLoadAt(cpu.bc(), cpu.a)
	case 0x03:
		gb.cpuOpIncrementRR(&cpu.b, &cpu.c)
	case 0x04:
		gb.cpuOpIncrement(&cpu.b)
	case 0x05:
		gb.cpuOpDecrement(&cpu.b)
	case 0x06:
		gb.cpuOpLoad(&cpu.b, gb.cpuFetch())
	case 0x07:
		gb.cpuOpRotateLeftCarry(&cpu.a)
	case 0x08:
		gb.cpuOpLoadAt16(gb.cpuFetch16(), cpu.sp)
	case 0x09:
		gb.cpuOpAddRR(&cpu.h, &cpu.l, cpu.bc())
	case 0x0A:
		gb.cpuOpLoad(&cpu.a, gb.fetchAt(cpu.bc()))
	case 0x0B:
		gb.cpuOpDecrementRR(&cpu.b, &cpu.c)
	case 0x0C:
		gb.cpuOpIncrement(&cpu.c)
	case 0x0D:
		gb.cpuOpDecrement(&cpu.c)
	case 0x0E:
		gb.cpuOpLoad(&cpu.c, gb.cpuFetch())
	case 0x0F:
		gb.cpuOpRotateRightCarry(&cpu.a)
	case 0x10:
		gb.cpuOpStop()
	case 0x11:
		gb.cpuOpLoadRR(&cpu.d, &cpu.e, gb.cpuFetch16())
	case 0x12:
		gb.cpuOpLoadAt(cpu.de(), cpu.a)
	case 0x13:
		gb.cpuOpIncrementRR(&cpu.d, &cpu.e)
	case 0x14:
		gb.cpuOpIncrement(&cpu.d)
	case 0x15:
		gb.cpuOpDecrement(&cpu.d)
	case 0x16:
		gb.cpuOpLoad(&cpu.d, gb.cpuFetch())
	case 0x17:
		gb.cpuOpRotateLeft(&cpu.a)
	case 0x18:
		gb.cpuOpJumpRel(int(gb.cpuFetchSigned()))
	case 0x19:
		gb.cpuOpAddRR(&cpu.h, &cpu.l, cpu.de())
	case 0x1A:
		gb.cpuOpLoad(&cpu.a, gb.fetchAt(cpu.de()))
	case 0x1B:
		gb.cpuOpDecrementRR(&cpu.d, &cpu.e)
	case 0x1C:
		gb.cpuOpIncrement(&cpu.e)
	case 0x1D:
		gb.cpuOpDecrement(&cpu.e)
	case 0x1E:
		gb.cpuOpLoad(&cpu.e, gb.cpuFetch())
	case 0x1F:
		gb.cpuOpRotateRight(&cpu.a)
	case 0x20:
		gb.cpuOpJumpRelFlag(!cpu.zf(), int(gb.cpuFetchSigned()))
	case 0x21:
		gb.cpuOpLoadRR(&cpu.h, &cpu.l, gb.cpuFetch16())
	case 0x22:
		gb.cpuOpLoadAt(cpu.hl(), cpu.a)
		cpu.setHL(cpu.hl() + 1)
	case 0x23:
		gb.cpuOpIncrementRR(&cpu.h, &cpu.l)
	case 0x24:
		gb.cpuOpIncrement(&cpu.h)
	case 0x25:
		gb.cpuOpDecrement(&cpu.h)
	case 0x26:
		gb.cpuOpLoad(&cpu.h, gb.cpuFetch())
	case 0x27:
		gb.cpuOpDecimalAdjust(&cpu.a)
	case 0x28:
		gb.cpuOpJumpRelFlag(cpu.zf(), int(gb.cpuFetchSigned()))
	case 0x29:
		gb.cpuOpAddRR(&cpu.h, &cpu.l, cpu.hl())
	case 0x2A:
		gb.cpuOpLoad(&cpu.a, gb.fetchAt(cpu.hl()))
		cpu.setHL(cpu.hl() + 1)
	case 0x2B:
		gb.cpuOpDecrementRR(&cpu.h, &cpu.l)
	case 0x2C:
		gb.cpuOpIncrement(&cpu.l)
	case 0x2D:
		gb.cpuOpDecrement(&cpu.l)
	case 0x2E:
		gb.cpuOpLoad(&cpu.l, gb.cpuFetch())
	case 0x2F:
		gb.cpuOpBitwiseComplement(&cpu.a)
	case 0x30:
		gb.cpuOpJumpRelFlag(!cpu.cf(), int(gb.cpuFetchSigned()))
	case 0x31:
		gb.cpuOpLoad16(&cpu.sp, gb.cpuFetch16())
	case 0x32:
		gb.cpuOpLoadAt(cpu.hl(), cpu.a)
		cpu.setHL(cpu.hl() - 1)
	case 0x33:
		gb.cpuOpIncrement16(&cpu.sp)
	case 0x34:
		gb.cpuOpIncrementAt(cpu.hl())
	case 0x35:
		gb.cpuOpDecrementAt(cpu.hl())
	case 0x36:
		gb.cpuOpLoadAt(cpu.hl(), gb.cpuFetch())
	case 0x37:
		gb.cpuOpSetCarryFlag()
	case 0x38:
		gb.cpuOpJumpRelFlag(cpu.cf(), int(gb.cpuFetchSigned()))
	case 0x39:
		gb.cpuOpAddRR(&cpu.h, &cpu.l, cpu.sp)
	case 0x3A:
		gb.cpuOpLoad(&cpu.a, gb.fetchAt(cpu.hl()))
		cpu.setHL(cpu.hl() - 1)
	case 0x3B:
		gb.cpuOpDecrement16(&cpu.sp)
	case 0x3C:
		gb.cpuOpIncrement(&cpu.a)
	case 0x3D:
		gb.cpuOpDecrement(&cpu.a)
	case 0x3E:
		gb.cpuOpLoad(&cpu.a, gb.cpuFetch())
	case 0x3F:
		gb.cpuOpComplementCarryFlag()
	case 0x40:
		gb.cpuOpLoad(&cpu.b, cpu.b)
	case 0x41:
		gb.cpuOpLoad(&cpu.b, cpu.c)
	case 0x42:
		gb.cpuOpLoad(&cpu.b, cpu.d)
	case 0x43:
		gb.cpuOpLoad(&cpu.b, cpu.e)
	case 0x44:
		gb.cpuOpLoad(&cpu.b, cpu.h)
	case 0x45:
		gb.cpuOpLoad(&cpu.b, cpu.l)
	case 0x46:
		gb.cpuOpLoad(&cpu.b, gb.fetchAt(cpu.hl()))
	case 0x47:
		gb.cpuOpLoad(&cpu.b, cpu.a)
	case 0x48:
		gb.cpuOpLoad(&cpu.c, cpu.b)
	case 0x49:
		gb.cpuOpLoad(&cpu.c, cpu.c)
	case 0x4A:
		gb.cpuOpLoad(&cpu.c, cpu.d)
	case 0x4B:
		gb.cpuOpLoad(&cpu.c, cpu.e)
	case 0x4C:
		gb.cpuOpLoad(&cpu.c, cpu.h)
	case 0x4D:
		gb.cpuOpLoad(&cpu.c, cpu.l)
	case 0x4E:
		gb.cpuOpLoad(&cpu.c, gb.fetchAt(cpu.hl()))
	case 0x4F:
		gb.cpuOpLoad(&cpu.c, cpu.a)
	case 0x50:
		gb.cpuOpLoad(&cpu.d, cpu.b)
	case 0x51:
		gb.cpuOpLoad(&cpu.d, cpu.c)
	case 0x52:
		gb.cpuOpLoad(&cpu.d, cpu.d)
	case 0x53:
		gb.cpuOpLoad(&cpu.d, cpu.e)
	case 0x54:
		gb.cpuOpLoad(&cpu.d, cpu.h)
	case 0x55:
		gb.cpuOpLoad(&cpu.d, cpu.l)
	case 0x56:
		gb.cpuOpLoad(&cpu.d, gb.fetchAt(cpu.hl()))
	case 0x57:
		gb.cpuOpLoad(&cpu.d, cpu.a)
	case 0x58:
		gb.cpuOpLoad(&cpu.e, cpu.b)
	case 0x59:
		gb.cpuOpLoad(&cpu.e, cpu.c)
	case 0x5A:
		gb.cpuOpLoad(&cpu.e, cpu.d)
	case 0x5B:
		gb.cpuOpLoad(&cpu.e, cpu.e)
	case 0x5C:
		gb.cpuOpLoad(&cpu.e, cpu.h)
	case 0x5D:
		gb.cpuOpLoad(&cpu.e, cpu.l)
	case 0x5E:
		gb.cpuOpLoad(&cpu.e, gb.fetchAt(cpu.hl()))
	case 0x5F:
		gb.cpuOpLoad(&cpu.e, cpu.a)
	case 0x60:
		gb.cpuOpLoad(&cpu.h, cpu.b)
	case 0x61:
		gb.cpuOpLoad(&cpu.h, cpu.c)
	case 0x62:
		gb.cpuOpLoad(&cpu.h, cpu.d)
	case 0x63:
		gb.cpuOpLoad(&cpu.h, cpu.e)
	case 0x64:
		gb.cpuOpLoad(&cpu.h, cpu.h)
	case 0x65:
		gb.cpuOpLoad(&cpu.h, cpu.l)
	case 0x66:
		gb.cpuOpLoad(&cpu.h, gb.fetchAt(cpu.hl()))
	case 0x67:
		gb.cpuOpLoad(&cpu.h, cpu.a)
	case 0x68:
		gb.cpuOpLoad(&cpu.l, cpu.b)
	case 0x69:
		gb.cpuOpLoad(&cpu.l, cpu.c)
	case 0x6A:
		gb.cpuOpLoad(&cpu.l, cpu.d)
	case 0x6B:
		gb.cpuOpLoad(&cpu.l, cpu.e)
	case 0x6C:
		gb.cpuOpLoad(&cpu.l, cpu.h)
	case 0x6D:
		gb.cpuOpLoad(&cpu.l, cpu.l)
	case 0x6E:
		gb.cpuOpLoad(&cpu.l, gb.fetchAt(cpu.hl()))
	case 0x6F:
		gb.cpuOpLoad(&cpu.l, cpu.a)
	case 0x70:
		gb.cpuOpLoadAt(cpu.hl(), cpu.b)
	case 0x71:
		gb.cpuOpLoadAt(cpu.hl(), cpu.c)
	case 0x72:
		gb.cpuOpLoadAt(cpu.hl(), cpu.d)
	case 0x73:
		gb.cpuOpLoadAt(cpu.hl(), cpu.e)
	case 0x74:
		gb.cpuOpLoadAt(cpu.hl(), cpu.h)
	case 0x75:
		gb.cpuOpLoadAt(cpu.hl(), cpu.l)
	case 0x76:
		gb.cpuOpHalt()
	case 0x77:
		gb.cpuOpLoadAt(cpu.hl(), cpu.a)
	case 0x78:
		gb.cpuOpLoad(&cpu.a, cpu.b)
	case 0x79:
		gb.cpuOpLoad(&cpu.a, cpu.c)
	case 0x7A:
		gb.cpuOpLoad(&cpu.a, cpu.d)
	case 0x7B:
		gb.cpuOpLoad(&cpu.a, cpu.e)
	case 0x7C:
		gb.cpuOpLoad(&cpu.a, cpu.h)
	case 0x7D:
		gb.cpuOpLoad(&cpu.a, cpu.l)
	case 0x7E:
		gb.cpuOpLoad(&cpu.a, gb.fetchAt(cpu.hl()))
	case 0x7F:
		gb.cpuOpLoad(&cpu.a, cpu.a)
	case 0x80:
		gb.cpuOpAdd(&cpu.a, cpu.b, false)
	case 0x81:
		gb.cpuOpAdd(&cpu.a, cpu.c, false)
	case 0x82:
		gb.cpuOpAdd(&cpu.a, cpu.d, false)
	case 0x83:
		gb.cpuOpAdd(&cpu.a, cpu.e, false)
	case 0x84:
		gb.cpuOpAdd(&cpu.a, cpu.h, false)
	case 0x85:
		gb.cpuOpAdd(&cpu.a, cpu.l, false)
	case 0x86:
		gb.cpuOpAdd(&cpu.a, gb.fetchAt(cpu.hl()), false)
	case 0x87:
		gb.cpuOpAdd(&cpu.a, cpu.a, false)
	case 0x88:
		gb.cpuOpAdd(&cpu.a, cpu.b, true)
	case 0x89:
		gb.cpuOpAdd(&cpu.a, cpu.c, true)
	case 0x8A:
		gb.cpuOpAdd(&cpu.a, cpu.d, true)
	case 0x8B:
		gb.cpuOpAdd(&cpu.a, cpu.e, true)
	case 0x8C:
		gb.cpuOpAdd(&cpu.a, cpu.h, true)
	case 0x8D:
		gb.cpuOpAdd(&cpu.a, cpu.l, true)
	case 0x8E:
		gb.cpuOpAdd(&cpu.a, gb.fetchAt(cpu.hl()), true)
	case 0x8F:
		gb.cpuOpAdd(&cpu.a, cpu.a, true)
	case 0x90:
		gb.cpuOpSub(&cpu.a, cpu.b, false)
	case 0x91:
		gb.cpuOpSub(&cpu.a, cpu.c, false)
	case 0x92:
		gb.cpuOpSub(&cpu.a, cpu.d, false)
	case 0x93:
		gb.cpuOpSub(&cpu.a, cpu.e, false)
	case 0x94:
		gb.cpuOpSub(&cpu.a, cpu.h, false)
	case 0x95:
		gb.cpuOpSub(&cpu.a, cpu.l, false)
	case 0x96:
		gb.cpuOpSub(&cpu.a, gb.fetchAt(cpu.hl()), false)
	case 0x97:
		gb.cpuOpSub(&cpu.a, cpu.a, false)
	case 0x98:
		gb.cpuOpSub(&cpu.a, cpu.b, true)
	case 0x99:
		gb.cpuOpSub(&cpu.a, cpu.c, true)
	case 0x9A:
		gb.cpuOpSub(&cpu.a, cpu.d, true)
	case 0x9B:
		gb.cpuOpSub(&cpu.a, cpu.e, true)
	case 0x9C:
		gb.cpuOpSub(&cpu.a, cpu.h, true)
	case 0x9D:
		gb.cpuOpSub(&cpu.a, cpu.l, true)
	case 0x9E:
		gb.cpuOpSub(&cpu.a, gb.fetchAt(cpu.hl()), true)
	case 0x9F:
		gb.cpuOpSub(&cpu.a, cpu.a, true)
	case 0xA0:
		gb.cpuOpAnd(&cpu.a, cpu.b)
	case 0xA1:
		gb.cpuOpAnd(&cpu.a, cpu.c)
	case 0xA2:
		gb.cpuOpAnd(&cpu.a, cpu.d)
	case 0xA3:
		gb.cpuOpAnd(&cpu.a, cpu.e)
	case 0xA4:
		gb.cpuOpAnd(&cpu.a, cpu.h)
	case 0xA5:
		gb.cpuOpAnd(&cpu.a, cpu.l)
	case 0xA6:
		gb.cpuOpAnd(&cpu.a, gb.fetchAt(cpu.hl()))
	case 0xA7:
		gb.cpuOpAnd(&cpu.a, cpu.a)
	case 0xA8:
		gb.cpuOpXor(&cpu.a, cpu.b)
	case 0xA9:
		gb.cpuOpXor(&cpu.a, cpu.c)
	case 0xAA:
		gb.cpuOpXor(&cpu.a, cpu.d)
	case 0xAB:
		gb.cpuOpXor(&cpu.a, cpu.e)
	case 0xAC:
		gb.cpuOpXor(&cpu.a, cpu.h)
	case 0xAD:
		gb.cpuOpXor(&cpu.a, cpu.l)
	case 0xAE:
		gb.cpuOpXor(&cpu.a, gb.fetchAt(cpu.hl()))
	case 0xAF:
		gb.cpuOpXor(&cpu.a, cpu.a)
	case 0xB0:
		gb.cpuOpOr(&cpu.a, cpu.b)
	case 0xB1:
		gb.cpuOpOr(&cpu.a, cpu.c)
	case 0xB2:
		gb.cpuOpOr(&cpu.a, cpu.d)
	case 0xB3:
		gb.cpuOpOr(&cpu.a, cpu.e)
	case 0xB4:
		gb.cpuOpOr(&cpu.a, cpu.h)
	case 0xB5:
		gb.cpuOpOr(&cpu.a, cpu.l)
	case 0xB6:
		gb.cpuOpOr(&cpu.a, gb.fetchAt(cpu.hl()))
	case 0xB7:
		gb.cpuOpOr(&cpu.a, cpu.a)
	case 0xB8:
		gb.cpuOpCompare(&cpu.a, cpu.b)
	case 0xB9:
		gb.cpuOpCompare(&cpu.a, cpu.c)
	case 0xBA:
		gb.cpuOpCompare(&cpu.a, cpu.d)
	case 0xBB:
		gb.cpuOpCompare(&cpu.a, cpu.e)
	case 0xBC:
		gb.cpuOpCompare(&cpu.a, cpu.h)
	case 0xBD:
		gb.cpuOpCompare(&cpu.a, cpu.l)
	case 0xBE:
		gb.cpuOpCompare(&cpu.a, gb.fetchAt(cpu.hl()))
	case 0xBF:
		gb.cpuOpCompare(&cpu.a, cpu.a)
	case 0xC0:
		gb.cpuOpReturnFlag(!cpu.zf())
	case 0xC1:
		gb.cpu.setBC(gb.cpuPop())
	case 0xC2:
		gb.cpuOpJumpFlag(!cpu.zf(), gb.cpuFetch16())
	case 0xC3:
		gb.cpuOpJump(gb.cpuFetch16())
	case 0xC4:
		gb.cpuOpCallFlag(!cpu.zf(), gb.cpuFetch16())
	case 0xC5:
		gb.cpuOpPush(gb.cpu.bc())
	case 0xC6:
		gb.cpuOpAdd(&cpu.a, gb.cpuFetch(), false)
	case 0xC7:
		gb.cpuOpRestart(0x00)
	case 0xC8:
		gb.cpuOpReturnFlag(cpu.zf())
	case 0xC9:
		gb.cpuOpReturn()
	case 0xCA:
		gb.cpuOpJumpFlag(cpu.zf(), gb.cpuFetch16())
	case 0xCB:
		panic("invalid opcode")
	case 0xCC:
		gb.cpuOpCallFlag(cpu.zf(), gb.cpuFetch16())
	case 0xCD:
		gb.cpuOpCall(gb.cpuFetch16())
	case 0xCE:
		gb.cpuOpAdd(&cpu.a, gb.cpuFetch(), true)
	case 0xCF:
		gb.cpuOpRestart(0x08)
	case 0xD0:
		gb.cpuOpReturnFlag(!cpu.cf())
	case 0xD1:
		gb.cpu.setDE(gb.cpuPop())
	case 0xD2:
		gb.cpuOpJumpFlag(!cpu.cf(), gb.cpuFetch16())
	case 0xD3:
		gb.cpuOpUndefined()
	case 0xD4:
		gb.cpuOpCallFlag(!cpu.cf(), gb.cpuFetch16())
	case 0xD5:
		gb.cpuOpPush(cpu.de())
	case 0xD6:
		gb.cpuOpSub(&cpu.a, gb.cpuFetch(), false)
	case 0xD7:
		gb.cpuOpRestart(0x10)
	case 0xD8:
		gb.cpuOpReturnFlag(cpu.cf())
	case 0xD9:
		gb.cpuOpReturnInterrupt()
	case 0xDA:
		gb.cpuOpJumpFlag(cpu.cf(), gb.cpuFetch16())
	case 0xDB:
		gb.cpuOpUndefined()
	case 0xDC:
		gb.cpuOpCallFlag(cpu.cf(), gb.cpuFetch16())
	case 0xDD:
		gb.cpuOpUndefined()
	case 0xDE:
		gb.cpuOpSub(&cpu.a, gb.cpuFetch(), true)
	case 0xDF:
		gb.cpuOpRestart(0x18)
	case 0xE0:
		gb.cpuOpLoadAt(uint16(0xFF00)+uint16(gb.cpuFetch()), cpu.a)
	case 0xE1:
		gb.cpu.setHL(gb.cpuPop())
	case 0xE2:
		gb.cpuOpLoadAt(uint16(0xFF00)+uint16(cpu.c), cpu.a)
	case 0xE3:
		gb.cpuOpUndefined()
	case 0xE4:
		gb.cpuOpUndefined()
	case 0xE5:
		gb.cpuOpPush(cpu.hl())
	case 0xE6:
		gb.cpuOpAnd(&cpu.a, gb.cpuFetch())
	case 0xE7:
		gb.cpuOpRestart(0x20)
	case 0xE8:
		gb.cpuOpAddSP(gb.cpuFetchSigned())
	case 0xE9:
		gb.cpuOpJump(cpu.hl())
	case 0xEA:
		gb.cpuOpLoadAt(gb.cpuFetch16(), cpu.a)
	case 0xEB:
		gb.cpuOpUndefined()
	case 0xEC:
		gb.cpuOpUndefined()
	case 0xED:
		gb.cpuOpUndefined()
	case 0xEE:
		gb.cpuOpXor(&cpu.a, gb.cpuFetch())
	case 0xEF:
		gb.cpuOpRestart(0x28)
	case 0xF0:
		gb.cpuOpLoad(&cpu.a, gb.fetchAt(uint16(0xFF00)+uint16(gb.cpuFetch())))
	case 0xF1:
		gb.cpu.setAF(gb.cpuPop())
	case 0xF2:
		gb.cpuOpLoad(&cpu.a, gb.fetchAt(uint16(0xFF00)+uint16(cpu.c)))
	case 0xF3:
		cpu.ime = false
	case 0xF4:
		gb.cpuOpUndefined()
	case 0xF5:
		gb.cpuOpPush(cpu.af())
	case 0xF6:
		gb.cpuOpOr(&cpu.a, gb.cpuFetch())
	case 0xF7:
		gb.cpuOpRestart(0x30)
	case 0xF8:
		gb.cpu.setHL(uint16(int(cpu.sp) + int(gb.cpuFetchSigned())))
	case 0xF9:
		gb.cpu.sp = cpu.hl()
	case 0xFA:
		gb.cpuOpLoad(&cpu.a, gb.fetchAt(gb.cpuFetch16()))
	case 0xFB:
		cpu.ime = true
	case 0xFC:
		gb.cpuOpUndefined()
	case 0xFD:
		gb.cpuOpUndefined()
	case 0xFE:
		gb.cpuOpCompare(&cpu.a, gb.cpuFetch())
	case 0xFF:
		gb.cpuOpRestart(0x38)
	}
}

func (gb *Machine) cpuDispatchCB(op uint8) {
	var reg *uint8

	bit := (op / 8) % 8

	switch op % 8 {
	case 0:
		reg = &gb.cpu.b
	case 1:
		reg = &gb.cpu.c
	case 2:
		reg = &gb.cpu.d
	case 3:
		reg = &gb.cpu.e
	case 4:
		reg = &gb.cpu.h
	case 5:
		reg = &gb.cpu.l
	case 6:
		val := gb.fetchAt(gb.cpu.hl())
		reg = &val
	case 7:
		reg = &gb.cpu.a
	default:
		panic(op)
	}

	switch op & 0xF8 {
	case 0x00:
		gb.cpuOpCBRotateLeftCarry(reg)
	case 0x08:
		gb.cpuOpCBRotateRightCarry(reg)
	case 0x10:
		gb.cpuOpCBRotateLeft(reg)
	case 0x18:
		gb.cpuOpCBRotateRight(reg)
	case 0x20:
		gb.cpuOpCBArithmeticShiftLeft(reg)
	case 0x28:
		gb.cpuOpCBArithmeticShiftRight(reg)
	case 0x30:
		gb.cpuOpCBSwap(reg)
	case 0x38:
		gb.cpuOpCBLogicalShiftRight(reg)
	case 0x40, 0x48, 0x50, 0x58, 0x60, 0x68, 0x70, 0x78:
		gb.cpuOpCBBit(bit, reg)
	case 0x80, 0x88, 0x90, 0x98, 0xA0, 0xA8, 0xB0, 0xB8:
		gb.cpuOpCBBitReset(bit, reg)
	case 0xC0, 0xC8, 0xD0, 0xD8, 0xE0, 0xE8, 0xF0, 0xF8:
		gb.cpuOpCBBitSet(bit, reg)
	}

	switch op % 8 {
	case 6:
		gb.writeAt(gb.cpu.hl(), *reg)
	}
}
