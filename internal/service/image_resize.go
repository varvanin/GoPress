package service

import (
	"image"
	"math"
)

// Bilinear interpolation uses premultiplied channels so transparent PNG edges stay clean.
func resizeImage(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	scale := float64(size) / float64(max(b.Dx(), b.Dy()))
	width, height := max(1, int(math.Round(float64(b.Dx())*scale))), max(1, int(math.Round(float64(b.Dy())*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sy := max(0, min(float64(b.Dy()-1), (float64(y)+.5)*float64(b.Dy())/float64(height)-.5))
		y0 := int(sy)
		y1 := min(y0+1, b.Dy()-1)
		fy := sy - float64(y0)
		for x := 0; x < width; x++ {
			sx := max(0, min(float64(b.Dx()-1), (float64(x)+.5)*float64(b.Dx())/float64(width)-.5))
			x0 := int(sx)
			x1 := min(x0+1, b.Dx()-1)
			fx := sx - float64(x0)
			r0, g0, b0, a0 := src.At(b.Min.X+x0, b.Min.Y+y0).RGBA()
			r1, g1, b1, a1 := src.At(b.Min.X+x1, b.Min.Y+y0).RGBA()
			r2, g2, b2, a2 := src.At(b.Min.X+x0, b.Min.Y+y1).RGBA()
			r3, g3, b3, a3 := src.At(b.Min.X+x1, b.Min.Y+y1).RGBA()
			channels := [4][4]uint32{{r0, r1, r2, r3}, {g0, g1, g2, g3}, {b0, b1, b2, b3}, {a0, a1, a2, a3}}
			offset := dst.PixOffset(x, y)
			for i, c := range channels {
				value := (float64(c[0])*(1-fx)+float64(c[1])*fx)*(1-fy) + (float64(c[2])*(1-fx)+float64(c[3])*fx)*fy
				dst.Pix[offset+i] = uint8(math.Round(value / 257))
			}
		}
	}
	return dst
}
