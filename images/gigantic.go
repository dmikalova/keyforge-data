// Package images makes the card images that Master Vault does not have as
// one file. A gigantic creature has two half images, and the card search
// shows one tile.
package images

import (
	"image"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

// Tile is the size of a card image in the public bucket.
var Tile = image.Rect(0, 0, 300, 420)

// Gigantic joins the two halves of a gigantic creature into one card tile.
// Each Master Vault half is a portrait image with its content turned 90°
// clockwise. Put the base half left of the art half, turn the pair 90°
// counter-clockwise so that the text is upright, and scale it into the tile
// with transparent edges.
func Gigantic(base, art image.Image) *image.RGBA {
	w, h := base.Bounds().Dx(), base.Bounds().Dy()
	pair := image.NewRGBA(image.Rect(0, 0, 2*w, h))
	draw.Draw(pair, image.Rect(0, 0, w, h), base, base.Bounds().Min, draw.Src)
	xdraw.CatmullRom.Scale(pair, image.Rect(w, 0, 2*w, h), art, art.Bounds(), draw.Src, nil)
	upright := rotateLeft(pair)
	return fit(upright, Tile)
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
