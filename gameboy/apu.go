package gameboy

// APU implements the audio processing unit of the Gameboy.
type APU struct {
	square1 struct {
		sweep uint8

		duty   uint8
		length uint8

		envelope uint8
		volume   uint8

		expire     bool
		cfrequency uint16
		ifrequency uint16
	}
}

func (gb *Machine) stepAudio() {
}
