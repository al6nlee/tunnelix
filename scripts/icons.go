//go:build ignore

// Run from the repository root: go run scripts/icons.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	f, e := os.Open("assets/brand/tunnelix-logo.png")
	must(e)
	src, e := png.Decode(f)
	must(e)
	f.Close()
	crop := image.Rect(365, 270, 890, 715)
	render := func(size int, template bool) *image.NRGBA {
		out := image.NewNRGBA(image.Rect(0, 0, size, size))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				// Preserve aspect ratio with a square, transparent canvas.
				sx := crop.Min.X + x*crop.Dx()/size
				sy := crop.Min.Y + (y-(size-size*crop.Dy()/crop.Dx())/2)*crop.Dx()/size
				if sy < crop.Min.Y || sy >= crop.Max.Y {
					continue
				}
				r, g, b, _ := src.At(sx, sy).RGBA()
				rr, gg, bb := uint8(r>>8), uint8(g>>8), uint8(b>>8)
				low := min(rr, gg, bb)
				if low > 225 {
					continue
				}
				if template {
					out.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
				} else {
					out.SetNRGBA(x, y, color.NRGBA{rr, gg, bb, 255})
				}
			}
		}
		return out
	}
	save := func(path string, img image.Image) {
		f, e := os.Create(path)
		must(e)
		must(png.Encode(f, img))
		must(f.Close())
	}
	save("assets/brand/icon.png", render(512, false))
	save("assets/brand/tray.png", render(64, true))
	save("frontend/public/icon.png", render(128, false))
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
