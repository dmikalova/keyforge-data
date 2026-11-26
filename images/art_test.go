package images

import (
	"image"
	"image/color"
	"testing"
)

func TestBlankArtAndUnderlay(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	top := image.NewRGBA(Tile)
	// A frame at the bottom, and a transparent art box.
	for y := 300; y < 420; y++ {
		for x := range 300 {
			top.SetRGBA(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	if !BlankArt(top) {
		t.Fatal("the image has a blank art box")
	}
	out := Underlay(top, solid(red))
	if BlankArt(out) || out.RGBAAt(150, 150) != red || out.RGBAAt(150, 400).B != 255 {
		t.Errorf("underlay: art %v, frame %v", out.RGBAAt(150, 150), out.RGBAAt(150, 400))
	}
	if BlankArt(solid(red)) {
		t.Error("a full image has art")
	}
}
