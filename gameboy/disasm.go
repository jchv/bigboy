package gameboy

import (
	"fmt"
	"io"
)

type busReader struct {
	bus  IO
	addr uint16
}

var (
	regtable = [8]string{"b", "c", "d", "e", "h", "l", "(hl)", "a"}
	rp1table = [4]string{"bc", "de", "hl", "sp"}
	rp2table = [4]string{"bc", "de", "hl", "af"}
	cndtable = [4]string{"nz", "z", "nc", "c"}
	alutable = [8]string{"add a,", "adc a,", "sub a,", "sbc a,", "and", "xor", "or", "cp"}
	rottable = [8]string{"rlc", "rrc", "rl", "rr", "sla", "sra", "sll", "srl"}
)

func (r *busReader) safeRead(addr uint16) (b byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool

			if err, ok = r.(error); ok {
				return
			}

			err = fmt.Errorf("%s", r)
		}
	}()

	b = r.bus.Read(addr)

	return b, nil
}

func (r *busReader) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i], err = r.safeRead(r.addr)

		if err != nil {
			return
		}

		r.addr++
		n++
	}

	return
}

// BusReader creates an io.Reader that reads from GameBoy memory.
func BusReader(gb *Machine, addr uint16) io.Reader {
	return &busReader{gb, addr}
}

func fetch8(r io.Reader) byte {
	b := []byte{0x00}
	r.Read(b)
	return b[0]
}

func fetch16(r io.Reader) uint16 {
	b := []byte{0x00, 0x00}
	r.Read(b)
	return uint16(b[1])<<8 | uint16(b[0])
}

// Disassemble returns a string representing the opcode read from the reader.
func Disassemble(r io.Reader) string {
	op := fetch8(r)

	if op == 0xCB {
		return disassembleCB(fetch8(r), r)
	}

	return disassemble(op, r)
}

// dissassemble disassembles unprefixed ops. Based on a couple references:
// - http://www.z80.info/decoding.htm (for the overarching patterns)
// - http://pastraiser.com/cpu/gameboy/gameboy_opcodes.html (for the LR35902)
func disassemble(op byte, r io.Reader) string {
	x := op >> 6 & 0x7
	y := op >> 3 & 0x7
	z := op >> 0 & 0x7

	p := y >> 1
	q := y & 0x1

	switch {
	case x == 0:
		switch z {
		case 0:
			switch y {
			case 0:
				return "nop"
			case 1:
				return fmt.Sprintf("ld $%04x, sp", fetch16(r))
			case 2:
				return fmt.Sprintf("stop")
			case 3:
				return fmt.Sprintf("jr %+d", int8(fetch8(r)))
			case 4, 5, 6, 7:
				return fmt.Sprintf("jr %s, %+d", cndtable[y-4], int8(fetch8(r)))
			}
		case 1:
			switch q {
			case 0:
				return fmt.Sprintf("ld %s, $%04x", rp1table[p], fetch16(r))
			case 1:
				return fmt.Sprintf("add hl, $%04x", rp1table[p])
			}
		case 2:
			switch q {
			case 0:
				switch p {
				case 0:
					return fmt.Sprintf("ld (bc), a")
				case 1:
					return fmt.Sprintf("ld (de), a")
				case 2:
					return fmt.Sprintf("ld (hl+), a")
				case 3:
					return fmt.Sprintf("ld (hl-), a")
				}
			case 1:
				switch p {
				case 0:
					return fmt.Sprintf("ld a, (bc)")
				case 1:
					return fmt.Sprintf("ld a, (de)")
				case 2:
					return fmt.Sprintf("ld a, (hl+)")
				case 3:
					return fmt.Sprintf("ld a, (hl-)")
				}
			}
		case 3:
			switch q {
			case 0:
				return fmt.Sprintf("inc %s", rp1table[p])
			case 1:
				return fmt.Sprintf("dec %s", rp1table[p])
			}
		case 4:
			return fmt.Sprintf("inc %s", regtable[y])
		case 5:
			return fmt.Sprintf("dec %s", regtable[y])
		case 6:
			return fmt.Sprintf("ld %s, $%02x", regtable[y], fetch8(r))
		case 7:
			switch y {
			case 0:
				return "rlca"
			case 1:
				return "rrca"
			case 2:
				return "rla"
			case 3:
				return "rra"
			case 4:
				return "daa"
			case 5:
				return "cpl"
			case 6:
				return "scf"
			case 7:
				return "ccf"
			}
		}
	case x == 1:
		if z == 6 && y == 6 {
			return "halt"
		}

		return fmt.Sprintf("ld %s, %s", regtable[y], regtable[z])
	case x == 2:
		return fmt.Sprintf("%s %s", alutable[y], regtable[z])
	case x == 3:
		switch z {
		case 0:
			switch y {
			case 0, 1, 2, 3:
				return fmt.Sprintf("ret %s", cndtable[y])
			case 4:
				return fmt.Sprintf("ld ($ff%02x), a", fetch8(r))
			case 5:
				return fmt.Sprintf("add sp, %d", int8(fetch8(r)))
			case 6:
				return fmt.Sprintf("ld a, ($ff%02x)", fetch8(r))
			case 7:
				return fmt.Sprintf("ld hl, sp%+d", int8(fetch8(r)))
			}
		case 1:
			switch q {
			case 0:
				return fmt.Sprintf("pop %s", rp2table[p])
			case 1:
				switch p {
				case 0:
					return "ret"
				case 1:
					return "reti"
				case 2:
					return "jp (hl)"
				case 3:
					return "ld sp, hl"
				}
			}
		case 2:
			switch y {
			case 0, 1, 2, 3:
				return fmt.Sprintf("jp %s, $%04x", cndtable[y], fetch16(r))
			case 4:
				return fmt.Sprintf("ld (c), a")
			case 5:
				return fmt.Sprintf("ld ($%04x), a", fetch16(r))
			case 6:
				return fmt.Sprintf("ld a, (c)")
			case 7:
				return fmt.Sprintf("ld a, ($%04x)", fetch16(r))
			}
		case 3:
			switch y {
			case 0:
				return fmt.Sprintf("jp $%04x", fetch16(r))
			case 1, 2, 3, 4, 5:
				break
			case 6:
				return "di"
			case 7:
				return "ei"
			}
		case 4:
			switch y {
			case 0, 1, 2, 3:
				return fmt.Sprintf("call %s, $%04x", cndtable[p], fetch16(r))
			default:
				break
			}
		case 5:
			switch y {
			case 0, 2, 4, 6:
				return fmt.Sprintf("push %s", rp2table[p])
			case 1:
				return fmt.Sprintf("call $%04x", fetch16(r))
			default:
				break
			}
		case 6:
			return fmt.Sprintf("%s $%02x", alutable[y], fetch8(r))
		case 7:
			return fmt.Sprintf("rst $%04x", y<<3)
		}
	}

	return fmt.Sprintf("db $%02x", op)
}

func disassembleCB(op byte, r io.Reader) string {
	x := op >> 6 & 0x7
	y := op >> 3 & 0x7
	z := op >> 0 & 0x7

	switch x {
	case 0:
		return fmt.Sprintf("%s, %s", rottable[y], regtable[z])
	case 1:
		return fmt.Sprintf("bit %d, %s", y, regtable[z])
	case 2:
		return fmt.Sprintf("res %d, %s", y, regtable[z])
	case 3:
		return fmt.Sprintf("set %d, %s", y, regtable[z])
	}

	return fmt.Sprintf("db $%02x", op)
}
