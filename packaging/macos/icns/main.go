package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	sizes := []struct {
		typ  string
		size int
	}{
		{"icp4", 16},
		{"icp5", 32},
		{"icp6", 64},
		{"ic07", 128},
		{"ic08", 256},
		{"ic09", 512},
		{"ic10", 1024},
	}
	var body []byte
	for _, s := range sizes {
		pngBytes := pngOf(s.size)
		var entry bytes.Buffer
		entry.WriteString(s.typ)
		binary.Write(&entry, binary.BigEndian, uint32(8+len(pngBytes)))
		entry.Write(pngBytes)
		body = append(body, entry.Bytes()...)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+len(body)))
	out.Write(body)
	if err := os.WriteFile(os.Args[1], out.Bytes(), 0o644); err != nil {
		panic(err)
	}
}

func pngOf(size int) []byte {
	const ss = 4
	big := raster(size * ss)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	n := ss * ss
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a int
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					c := big.NRGBAAt(x*ss+sx, y*ss+sy)
					r += int(c.R)
					g += int(c.G)
					b += int(c.B)
					a += int(c.A)
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func raster(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	radius := size / 4
	bg := color.NRGBA{0x4f, 0x46, 0xe5, 0xff}
	fg := color.NRGBA{0xff, 0xff, 0xff, 0xff}
	cx, cy := size/2, size/2
	rad := size * 5 / 32
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !inRound(x, y, size, radius) {
				continue
			}
			img.SetNRGBA(x, y, bg)
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= rad*rad {
				img.SetNRGBA(x, y, fg)
			}
		}
	}
	return img
}

func inRound(x, y, size, radius int) bool {
	// Keep the corner test inside the rect. Pixel centers sit at x+0.5.
	fx, fy := x, y
	if fx < radius && fy < radius {
		dx, dy := fx-radius, fy-radius
		return dx*dx+dy*dy <= radius*radius
	}
	if fx >= size-radius && fy < radius {
		dx, dy := fx-(size-radius-1), fy-radius
		return dx*dx+dy*dy <= radius*radius
	}
	if fx < radius && fy >= size-radius {
		dx, dy := fx-radius, fy-(size-radius-1)
		return dx*dx+dy*dy <= radius*radius
	}
	if fx >= size-radius && fy >= size-radius {
		dx, dy := fx-(size-radius-1), fy-(size-radius-1)
		return dx*dx+dy*dy <= radius*radius
	}
	return true
}
