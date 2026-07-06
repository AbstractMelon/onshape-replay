package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// addBackground composites the source PNG (which may have an alpha channel)
// onto a solid background of the given hex color (e.g. "#ffffff").
// If bgHex is empty, white is used. Returns the opaque PNG bytes.
func addBackground(pngSrc []byte, bgHex string) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(pngSrc))
	if err != nil {
		return nil, fmt.Errorf("addBackground: decode: %w", err)
	}

	bg := parseHexColor(bgHex)
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)

	// Fill background.
	draw.Draw(dst, bounds, &image.Uniform{bg}, bounds.Min, draw.Src)
	// Composite source (with alpha) on top.
	draw.Draw(dst, bounds, src, bounds.Min, draw.Over)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("addBackground: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// parseHexColor parses a hex color string like "#ffffff" or "#fff" into
// an opaque color.RGBA. Returns white on parse failure.
func parseHexColor(s string) color.RGBA {
	if s == "" {
		return color.RGBA{255, 255, 255, 255}
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.RGBA{255, 255, 255, 255}
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

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
