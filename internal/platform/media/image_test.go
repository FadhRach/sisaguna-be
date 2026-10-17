package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func pngBytes(w, h int, c color.Color) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestProcessImage(t *testing.T) {
	t.Run("large png is resized to 1600 and re-encoded as jpeg with a thumbnail", func(t *testing.T) {
		out, err := ProcessImage(pngBytes(3200, 1600, color.RGBA{200, 50, 50, 255}))
		if err != nil {
			t.Fatal(err)
		}
		if out.Width != 1600 || out.Height != 800 {
			t.Errorf("size = %dx%d", out.Width, out.Height)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(out.Main))
		if err != nil || cfg.Width != 1600 {
			t.Errorf("main not a 1600px jpeg: %v %v", cfg, err)
		}
		tc, err := jpeg.DecodeConfig(bytes.NewReader(out.Thumb))
		if err != nil || tc.Width != 400 {
			t.Errorf("thumb not a 400px jpeg: %v %v", tc, err)
		}
	})

	t.Run("small image keeps its size", func(t *testing.T) {
		out, err := ProcessImage(pngBytes(100, 80, color.White))
		if err != nil || out.Width != 100 || out.Height != 80 {
			t.Errorf("%+v %v", out, err)
		}
	})

	t.Run("type is decided by content, not by name", func(t *testing.T) {
		for name, data := range map[string][]byte{
			"text": []byte("<html>bukan gambar</html>"),
			"gif":  []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"),
			"mp4":  append([]byte{0, 0, 0, 0x18}, []byte("ftypmp42\x00\x00\x00\x00mp42isom")...),
		} {
			if _, err := ProcessImage(data); err != ErrUnsupportedType {
				t.Errorf("%s: got %v, want ErrUnsupportedType", name, err)
			}
		}
	})

	t.Run("truncated png is corrupt", func(t *testing.T) {
		data := pngBytes(50, 50, color.Black)
		if _, err := ProcessImage(data[:len(data)/2]); err != ErrCorruptImage {
			t.Errorf("got %v", err)
		}
	})
}

func TestPathsPrivateDiffersFromPublic(t *testing.T) {
	data := []byte("sama")
	pubSha, rel, thumb := Paths(Public, data)
	privSha, _, _ := Paths(Private, data)
	if pubSha == privSha {
		t.Error("private and public hash must differ")
	}
	if rel != pubSha[:2]+"/"+pubSha+".jpg" || thumb != pubSha[:2]+"/"+pubSha+"_thumb.jpg" {
		t.Errorf("rel=%s thumb=%s", rel, thumb)
	}
}
