package gameboy

func bit(b uint8) uint8 {
	return 1 << b
}

func setBit(word *uint8, bit uint8, set bool) {
	if set {
		*word |= (1 << bit)
	} else {
		*word &= ^(1 << bit)
	}
}

func getBit(word uint8, bit uint8, set *bool) {
	*set = word&(1<<bit) != 0
}

func wide(hi, lo uint8) uint16 {
	return uint16(hi)<<8 | uint16(lo)
}

func narrow(n uint16) (hi, lo uint8) {
	return uint8(n << 8), uint8(n & 0xff)
}
