package render

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const overlayPadding = 8

// overlayFeatureText draws the feature name as white text with a black
// drop shadow in the top-left corner of the PNG frame. Returns the
// modified PNG bytes.
func overlayFeatureText(pngSrc []byte, text string) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(pngSrc))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	face := basicfont.Face7x13
	dot := fixed.P(overlayPadding, overlayPadding+face.Metrics().Ascent.Ceil())

	// White text with black shadow for readability on any background.
	shadow := color.RGBA{0, 0, 0, 200}
	white := color.RGBA{255, 255, 255, 255}

	// Draw shadow offset by 1 pixel.
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(shadow),
		Face: face,
		Dot:  fixed.P(dot.X.Ceil()+1, dot.Y.Ceil()+1),
	}
	d.DrawString(text)

	// Draw white text.
	d.Dot = dot
	d.Src = image.NewUniform(white)
	d.DrawString(text)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
