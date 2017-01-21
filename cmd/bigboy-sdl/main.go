package main

import (
	"flag"
	"io/ioutil"
	"log"
	"runtime"
	"time"

	"github.com/johnwchadwick/bigboy/gameboy"
	"github.com/veandco/go-sdl2/sdl"
)

const (
	w = 160
	h = 144

	cyclesPerSecond = 4194304
)

var (
	rom        []byte
	trace      bool
	useBootrom bool
)

func init() {
	var err error

	runtime.LockOSThread()

	// Parse command line
	flag.BoolVar(&trace, "trace", false, "enables instruction tracing")
	flag.BoolVar(&useBootrom, "bootrom", true, "start in bootrom")
	flag.Parse()

	// Load ROM
	if romFile := flag.Arg(0); romFile != "" {
		rom, err = ioutil.ReadFile(romFile)

		if err != nil {
			panic(err)
		}

		log.Println("loaded rom", romFile)
	}
}

func main() {
	var event sdl.Event
	var pad gameboy.Gamepad

	// TODO: detect cart type
	cart := gameboy.NewMBC1Cartridge(rom)
	gb := gameboy.NewMachine(cart, useBootrom)
	gb.SetTrace(trace)

	// Create window
	sdl.Init(sdl.INIT_EVERYTHING)
	window, err := sdl.CreateWindow("big boy",
		sdl.WINDOWPOS_UNDEFINED,
		sdl.WINDOWPOS_UNDEFINED,
		w*3, h*3,
		sdl.WINDOW_SHOWN)
	if err != nil {
		panic(err)
	}
	defer window.Destroy()

	// Get screen
	screen, err := window.GetSurface()
	if err != nil {
		panic(err)
	}

	buffer, err := sdl.CreateRGBSurface(0, 160, 144, 32, 0, 0, 0, 0)
	if err != nil {
		panic(err)
	}

	framebuf := gb.GetFrameBuffer()
	frameTime := time.Now()

MainLoop:
	for {
		for event = sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch t := event.(type) {
			case *sdl.QuitEvent:
				break MainLoop
			case *sdl.KeyDownEvent:
				switch t.Keysym.Sym {
				case sdl.K_ESCAPE:
					break MainLoop
				case sdl.K_DOWN:
					pad.Down = true
				case sdl.K_UP:
					pad.Up = true
				case sdl.K_LEFT:
					pad.Left = true
				case sdl.K_RIGHT:
					pad.Right = true
				case sdl.K_RETURN, sdl.K_RETURN2:
					pad.Start = true
				case sdl.K_SPACE:
					pad.Select = true
				case sdl.K_z:
					pad.B = true
				case sdl.K_x:
					pad.A = true
				}
				if t.Keysym.Sym == sdl.K_ESCAPE {
					break MainLoop
				}

			case *sdl.KeyUpEvent:
				switch t.Keysym.Sym {
				case sdl.K_DOWN:
					pad.Down = false
				case sdl.K_UP:
					pad.Up = false
				case sdl.K_LEFT:
					pad.Left = false
				case sdl.K_RIGHT:
					pad.Right = false
				case sdl.K_RETURN, sdl.K_RETURN2:
					pad.Start = false
				case sdl.K_SPACE:
					pad.Select = false
				case sdl.K_z:
					pad.B = false
				case sdl.K_x:
					pad.A = false
				}
			}
		}

		// Clear screen.
		for i := range framebuf {
			framebuf[i] = 0xFFFFFFFF
		}

		// Step frame.
		gb.UpdatePad(pad)
		frameCycles := gb.StepFrame()

		// Sleep to simulate timing.
		frameDuration := (time.Duration(frameCycles) * time.Second) / cyclesPerSecond
		frameTime = frameTime.Add(frameDuration)
		currTime := time.Now()
		if frameTime.After(currTime) {
			time.Sleep(frameTime.Sub(currTime))
		}

		// Draw framebuffer to buffer.
		err = buffer.Lock()
		pixels := buffer.Data()

		pitch := int(buffer.Pitch)
		if err != nil {
			panic(err)
		}

		for x := 0; x < w; x++ {
			for y := 0; y < h; y++ {
				(*[w * h]uint32)(pixels)[y*(pitch/4)+x] = framebuf[y*w+x]
			}
		}

		buffer.Unlock()

		// Blit to screen.
		buffer.BlitScaled(&sdl.Rect{0, 0, w, h}, screen, &sdl.Rect{0, 0, screen.W, screen.H})
		window.UpdateSurface()
	}

	sdl.Quit()
}
