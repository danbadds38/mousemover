//go:build ignore

// Command generate writes the two tray icons: a filled circle for the
// active state and a hollow grey ring for the disabled state.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const size = 32

func main() {
	write("assets/active.ico", draw(color.RGBA{0x2E, 0x9B, 0xF0, 0xFF}, true))
	write("assets/idle.ico", draw(color.RGBA{0x88, 0x88, 0x88, 0xFF}, false))
}

func draw(c color.RGBA, filled bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center, outer, inner := float64(size)/2, float64(size)/2-2, float64(size)/2-7
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-center, float64(y)+0.5-center
			d := math.Hypot(dx, dy)
			if d > outer || (!filled && d < inner) {
				continue
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// write wraps a PNG in a single-image ICO container, which Windows accepts.
func write(path string, img *image.RGBA) {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		log.Fatal(err)
	}
	var out bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), 1 image
	binary.Write(&out, binary.LittleEndian, []uint16{0, 1, 1})
	// ICONDIRENTRY
	out.Write([]byte{size, size, 0, 0})
	binary.Write(&out, binary.LittleEndian, uint16(1))  // colour planes
	binary.Write(&out, binary.LittleEndian, uint16(32)) // bits per pixel
	binary.Write(&out, binary.LittleEndian, uint32(pngBuf.Len()))
	binary.Write(&out, binary.LittleEndian, uint32(22)) // offset past the headers
	out.Write(pngBuf.Bytes())
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}
