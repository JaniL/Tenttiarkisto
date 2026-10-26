package pdfmerge

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func pngImage(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImageToPDFProducesValidPDF(t *testing.T) {
	imgs := [][]byte{
		pngImage(t, 100, 200, color.RGBA{R: 255, A: 255}),
		pngImage(t, 300, 150, color.RGBA{G: 255, A: 255}),
	}
	pdf, err := ImageToPDF(imgs)
	if err != nil {
		t.Fatalf("ImageToPDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("output does not start with %%PDF- header")
	}
	if !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Errorf("output missing %%EOF trailer")
	}
	// two images -> expect two pages ("/Type /Page" appears twice)
	n := bytes.Count(pdf, []byte("/Type /Page")) - bytes.Count(pdf, []byte("/Type /Pages"))
	if n != 2 {
		t.Errorf("expected 2 pages, got %d", n)
	}
}

func TestImageToPDFRejectsEmpty(t *testing.T) {
	if _, err := ImageToPDF(nil); err == nil {
		t.Fatal("expected error for no images")
	}
}

func TestImageToPDFRejectsGarbage(t *testing.T) {
	if _, err := ImageToPDF([][]byte{{0x00, 0x01, 0x02}}); err == nil {
		t.Fatal("expected error for invalid image data")
	}
}
