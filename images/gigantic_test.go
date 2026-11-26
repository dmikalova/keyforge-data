package images

import (
	"image"
	"image/color"
	"testing"
)

func solid(c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 300, 420))
	for y := range 420 {
		for x := range 300 {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestGigantic(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	out := Gigantic(solid(red), solid(blue))
	if out.Bounds() != Tile {
		t.Fatalf("bounds %v, want %v", out.Bounds(), Tile)
	}
	// The pair is 420×600 after the turn, so it fits as 294×420. The base
	// half (left before the turn) is at the bottom after the turn.
	if got := out.RGBAAt(150, 400); got != red {
		t.Errorf("bottom = %v, want the base half", got)
	}
	if got := out.RGBAAt(150, 20); got != blue {
		t.Errorf("top = %v, want the art half", got)
	}
	if got := out.RGBAAt(0, 200); got.A != 0 {
		t.Errorf("edge = %v, want transparent", got)
	}
}
