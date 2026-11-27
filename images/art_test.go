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

// paint fills a rectangle of an image with one color.
func paint(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// TestGreyFrame checks the grey frame of a house-less maverick render, such
// as the Menagerie image of The Grim Reaper: no colored house badge and no
// colored house stripe at the bottom.
func TestGreyFrame(t *testing.T) {
	grey := color.RGBA{R: 146, G: 141, B: 123, A: 255}
	purple := color.RGBA{R: 110, G: 40, B: 160, A: 255}
	frame := func() *image.RGBA {
		img := image.NewRGBA(Tile)
		paint(img, Tile, grey)
		return img
	}
	if !GreyFrame(frame()) {
		t.Error("a grey frame is a grey frame")
	}
	// A muted house badge, such as Shadows, has a colored bottom stripe.
	badge := frame()
	paint(badge, image.Rect(20, 20, 40, 40), purple)
	if GreyFrame(badge) {
		t.Error("a colored house badge is not a grey frame")
	}
	stripe := frame()
	paint(stripe, image.Rect(60, 396, 240, 410), purple)
	if GreyFrame(stripe) {
		t.Error("a colored house stripe is not a grey frame")
	}
	// The check scales to the image size.
	big := image.NewRGBA(image.Rect(0, 0, 600, 840))
	paint(big, big.Bounds(), grey)
	paint(big, image.Rect(40, 40, 80, 80), purple)
	if GreyFrame(big) {
		t.Error("a colored badge on a large image is not a grey frame")
	}
}
