// Package images makes the card images that Master Vault does not have as
// one file. A gigantic creature has two half images, and the card search
// shows one tile.
package images

import (
	"image"
	"image/color"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

// Tile is the size of a card image in the public bucket.
var Tile = image.Rect(0, 0, 300, 420)

// Gigantic joins the two halves of a gigantic creature into one card tile.
// Each Master Vault half is a portrait image with its content turned 90°
// clockwise. Put the base half left of the art half, overlap them by the
// art columns that both halves have (Overlap), turn the pair 90°
// counter-clockwise so that the text is upright, and scale it into the tile
// with transparent edges.
func Gigantic(base, art image.Image) *image.RGBA {
	w, h := base.Bounds().Dx(), base.Bounds().Dy()
	// Scale the art half to the size of the base half first, so that the
	// overlap compares the same pixels.
	a := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(a, a.Bounds(), art, art.Bounds(), draw.Src, nil)
	k := Overlap(base, a)
	pair := image.NewRGBA(image.Rect(0, 0, 2*w-k, h))
	draw.Draw(pair, image.Rect(0, 0, w, h), base, base.Bounds().Min, draw.Src)
	draw.Draw(pair, image.Rect(w-k, 0, 2*w-k, h), a, image.Point{}, draw.Over)
	upright := rotateLeft(pair)
	return fit(upright, Tile)
}

// maxOverlap is the most columns that Overlap tries. Master Vault's halves
// repeat about 6 columns of art at the join.
const maxOverlap = 16

// overlapLimit is the largest mean difference of the RGB sum (0 to 765) of a
// pixel at which the columns count as the same art. Image compression keeps
// repeated art from matching exactly.
const overlapLimit = 60

// Overlap returns the number of columns at the left edge of the art half
// that repeat the columns at the right edge of the base half: the overlap
// with the smallest mean difference, or 0 if no overlap is close enough. It
// skips the top and bottom tenth, where the rounded card corners are.
func Overlap(base, art image.Image) int {
	bb, ab := base.Bounds(), art.Bounds()
	h := min(bb.Dy(), ab.Dy())
	y0, y1 := h/10, h-h/10
	best, bestDiff := 0, float64(overlapLimit)
	for k := 1; k <= maxOverlap && k <= bb.Dx() && k <= ab.Dx(); k++ {
		sum, n := 0, 0
		for c := range k {
			for y := y0; y < y1; y++ {
				sum += rgbDiff(
					base.At(bb.Max.X-k+c, bb.Min.Y+y),
					art.At(ab.Min.X+c, ab.Min.Y+y),
				)
				n++
			}
		}
		if d := float64(sum) / float64(n); d < bestDiff {
			best, bestDiff = k, d
		}
	}
	return best
}

// rgbDiff returns the sum of the absolute differences of the red, green,
// and blue values (0 to 255 each) of two colors.
func rgbDiff(a, b color.Color) int {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	d := func(x, y uint32) int {
		if x > y {
			return int(x-y) >> 8
		}
		return int(y-x) >> 8
	}
	return d(ar, br) + d(ag, bg) + d(ab, bb)
}

// rotateLeft turns an image 90° counter-clockwise.
func rotateLeft(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			out.SetRGBA(y, b.Dx()-1-x, src.RGBAAt(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// fit scales src to fit in r, keeps the aspect ratio, and centers it.
func fit(src image.Image, r image.Rectangle) *image.RGBA {
	sb := src.Bounds()
	w, h := r.Dx(), sb.Dy()*r.Dx()/sb.Dx()
	if h > r.Dy() {
		w, h = sb.Dx()*r.Dy()/sb.Dy(), r.Dy()
	}
	dst := image.NewRGBA(r)
	x, y := (r.Dx()-w)/2, (r.Dy()-h)/2
	xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+w, y+h), src, sb, draw.Src, nil)
	return dst
}
