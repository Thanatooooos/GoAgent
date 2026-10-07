package imageevidence

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPrepareProcessingImageBoundsDimensions(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 3000, 10))
	source.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil { t.Fatal(err) }
	prepared, mimeType, err := prepareProcessingImage(encoded.Bytes(), "image/png")
	if err != nil { t.Fatal(err) }
	config, _, err := image.DecodeConfig(bytes.NewReader(prepared))
	if err != nil { t.Fatal(err) }
	if mimeType != "image/png" || config.Width > maxProcessingEdge || config.Height > maxProcessingEdge || len(prepared) > maxProcessingBytes {
		t.Fatalf("unbounded processing copy: mime=%s width=%d height=%d bytes=%d", mimeType, config.Width, config.Height, len(prepared))
	}
}
