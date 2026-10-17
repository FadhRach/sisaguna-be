// Package media berisi pemrosesan gambar, penyimpanan di disk, dan penyusun URL.
// Tidak tahu soal HTTP maupun database.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // decoder untuk image.Decode

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp" // decoder WebP
)

const (
	maxSide      = 1600
	thumbSide    = 400
	jpegQuality  = 80
	maxPixels    = 50_000_000 // tolak gambar raksasa sebelum di-decode penuh (decompression bomb)
	sniffBytes   = 512
	MaxImageSize = 8 << 20
)

var (
	ErrUnsupportedType = errors.New("tipe file tidak didukung, gunakan JPEG, PNG, atau WebP")
	ErrImageTooLarge   = errors.New("dimensi gambar terlalu besar")
	ErrCorruptImage    = errors.New("file gambar rusak")
)

type Image struct {
	Main, Thumb   []byte
	Width, Height int
}

// ProcessImage memeriksa tipe dari isi byte, memperkecil ke sisi terpanjang 1600 px,
// dan menyimpan ulang sebagai JPEG kualitas 80. Encode ulang membuang EXIF (termasuk GPS);
// orientasi EXIF diterapkan dulu supaya foto tidak miring.
func ProcessImage(data []byte) (*Image, error) {
	if !isSupported(data) {
		return nil, ErrUnsupportedType
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrCorruptImage
	}
	if cfg.Width*cfg.Height > maxPixels {
		return nil, ErrImageTooLarge
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, ErrCorruptImage
	}
	if format != "jpeg" {
		img = flattenOnWhite(img) // JPEG tidak punya alpha
	}
	if b := img.Bounds(); b.Dx() > maxSide || b.Dy() > maxSide {
		img = imaging.Fit(img, maxSide, maxSide, imaging.Lanczos)
	}
	main, err := encodeJPEG(img)
	if err != nil {
		return nil, err
	}
	thumb, err := encodeJPEG(imaging.Fit(img, thumbSide, thumbSide, imaging.Lanczos))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	return &Image{Main: main, Thumb: thumb, Width: b.Dx(), Height: b.Dy()}, nil
}

func isSupported(data []byte) bool {
	head := data
	if len(head) > sniffBytes {
		head = head[:sniffBytes]
	}
	switch mime := httpDetect(head); mime {
	case "image/jpeg", "image/png", "image/webp":
		return true
	}
	return false
}

func flattenOnWhite(img image.Image) image.Image {
	bg := imaging.New(img.Bounds().Dx(), img.Bounds().Dy(), color.White)
	return imaging.Overlay(bg, img, image.Pt(0, 0), 1)
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("media encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}
