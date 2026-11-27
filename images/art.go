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

// The boxes of a 300×420 card image that show the house: the house badge at
// the top left, and the colored stripe at the bottom.
var (
	badgeBox  = image.Rect(10, 8, 62, 60)
	stripeBox = image.Rect(60, 396, 240, 410)
)

// The largest shares of colored pixels in the badge box and in the stripe
// box of a grey frame. On 2026-11-27 the grey Menagerie image of The Grim
// Reaper had 0.1% and 0%. The most muted house badges (Shadows) had 4%, and
// the most muted house stripes (Unfathomable tokens) had 0.7%, so a grey
// frame must pass both limits.
const (
	greyBadge  = 0.015
	greyStripe = 0.02
)

// GreyFrame reports whether a card image has the grey frame of a house-less
// maverick render: no colored house badge and no colored house stripe.
// Master Vault sends some normal printings with this image, for example The
// Grim Reaper in Menagerie. A card without a house, a reference card, and
// an anomaly have a grey frame on purpose, so check only cards with a
// house.
func GreyFrame(img image.Image) bool {
	return colored(img, badgeBox) < greyBadge && colored(img, stripeBox) < greyStripe
}

// colored returns the share of the opaque pixels in a box of a 300×420
// image that have a clear color: a saturation above 0.35 and not dark. The
// box scales to the image size.
func colored(img image.Image, box image.Rectangle) float64 {
	b := img.Bounds()
	n, sat := 0, 0
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			r, g, bl, a := img.At(
				b.Min.X+x*b.Dx()/Tile.Dx(),
				b.Min.Y+y*b.Dy()/Tile.Dy(),
			).RGBA()
			if a < 0x8000 {
				continue
			}
			hi, lo := max(r, g, bl)>>8, min(r, g, bl)>>8
			n++
			if hi > 50 && float64(hi-lo)/float64(hi) > 0.35 {
				sat++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return float64(sat) / float64(n)
}
