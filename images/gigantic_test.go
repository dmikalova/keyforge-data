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

// columns returns a 300×420 image whose column x has the gray level
// from a hash of start+x, so that columns do not repeat, as in card art.
func columns(start int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 300, 420))
	for y := range 420 {
		for x := range 300 {
			v := mix(uint32(start + x))
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

// TestOverlap checks that the art half's first columns that repeat the base
// half's last columns are found, so the halves overlap by them.
func TestOverlap(t *testing.T) {
	if got := Overlap(columns(0), columns(294)); got != 6 {
		t.Errorf("overlap = %d, want 6", got)
	}
	if got := Overlap(columns(0), columns(300)); got != 0 {
		t.Errorf("overlap of halves that do not repeat = %d, want 0", got)
	}
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	if got := Overlap(solid(red), solid(blue)); got != 0 {
		t.Errorf("overlap of different halves = %d, want 0", got)
	}
}

// mix returns a gray level that looks random for each n.
func mix(n uint32) uint8 {
	n ^= n >> 16
	n *= 0x45d9f3b
	n ^= n >> 16
	n *= 0x45d9f3b
	n ^= n >> 16
	return uint8(n)
}
