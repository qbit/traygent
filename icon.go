package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var myFont, _ = opentype.Parse(goregular.TTF)
var fontFace, _ = opentype.NewFace(myFont, &opentype.FaceOptions{
	Size:    230,
	DPI:     72,
	Hinting: font.HintingNone,
})

type myIcon struct {
	data *image.RGBA
}

func (m *myIcon) Name() string {
	return "tagent.png"
}

func (m *myIcon) Content() []byte {
	buf := new(bytes.Buffer)
	_ = png.Encode(buf, m.data)
	return buf.Bytes()
}

func buildImage(length int, locked bool) *myIcon {
	i := &myIcon{}

	size := 256
	i.data = image.NewRGBA(image.Rect(0, 0, size, size))

	co := color.RGBA{A: 255}

	d := &font.Drawer{
		Dst:  i.data,
		Src:  image.NewUniform(co),
		Face: fontFace,
		Dot:  fixed.P(60, 210),
	}

	var r []rune
	if !locked {
		r = []rune(fmt.Sprintf("%d", length))
	} else {
		r = []rune("🔒")
	}
	l := string(r)

	d.DrawString(l)

	return i
}
