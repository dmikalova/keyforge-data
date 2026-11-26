package images

import (
	"image"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

// artPoints are points inside the art box of a 300×420 card image.
var artPoints = []image.Point{{150, 90}, {150, 150}, {100, 120}, {200, 120}}

// BlankArt reports whether a card image has no art. Master Vault sends some
// images with a transparent art box, for example Mercy, Malkin Queen in
// Vault Masters 2023. The points scale to the image size.
func BlankArt(img image.Image) bool {
	b := img.Bounds()
	for _, p := range artPoints {
		x := b.Min.X + p.X*b.Dx()/Tile.Dx()
		y := b.Min.Y + p.Y*b.Dy()/Tile.Dy()
		if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
			return false
		}
	}
	return true
}

// Underlay puts the art of another printing of the same card under an
// image with a blank art box. The frame, the text, and the stats of the top
// image stay.
func Underlay(top, art image.Image) *image.RGBA {
	out := image.NewRGBA(Tile)
	xdraw.CatmullRom.Scale(out, Tile, art, art.Bounds(), draw.Src, nil)
	xdraw.CatmullRom.Scale(out, Tile, top, top.Bounds(), draw.Over, nil)
	return out
}
