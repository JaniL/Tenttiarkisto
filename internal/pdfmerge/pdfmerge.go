// Package pdfmerge combines uploaded images (JPEG/PNG) into a single
// PDF document, one image per page. This replaces the legacy
// ImageMagick/rmagick conversion from the Rails application.
package pdfmerge

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/go-pdf/fpdf"
)

// ImageToPDF merges the given images into one PDF. Pixel dimensions are
// treated as points (72 dpi); PDF viewers scale pages to fit anyway.
func ImageToPDF(images [][]byte) ([]byte, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("no images given")
	}

	sizes := make([][2]float64, len(images))
	types := make([]string, len(images))
	for i, data := range images {
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("image %d: %w", i, err)
		}
		switch format {
		case "jpeg":
			types[i] = "JPG"
		case "png":
			types[i] = "PNG"
		default:
			return nil, fmt.Errorf("image %d: unsupported format %q (only JPEG/PNG)", i, format)
		}
		sizes[i] = [2]float64{float64(cfg.Width), float64(cfg.Height)}
	}

	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "P",
		UnitStr:        "pt",
		SizeStr:        "",
		Size:           fpdf.SizeType{Wd: sizes[0][0], Ht: sizes[0][1]},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	for i, data := range images {
		pdf.AddPageFormat("P", fpdf.SizeType{Wd: sizes[i][0], Ht: sizes[i][1]})
		name := fmt.Sprintf("img%d", i)
		info := pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: types[i]}, bytes.NewReader(data))
		if info == nil {
			return nil, fmt.Errorf("register image %d: failed", i)
		}
		pdf.ImageOptions(name, 0, 0, sizes[i][0], sizes[i][1], false,
			fpdf.ImageOptions{ImageType: types[i]}, -1, "")
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output: %w", err)
	}
	return buf.Bytes(), nil
}
