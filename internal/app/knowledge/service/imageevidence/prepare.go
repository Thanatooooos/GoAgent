package imageevidence

import (
	"bytes"
	"fmt"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
)

const maxProcessingPixels = 40_000_000
const maxProcessingEdge = 2048
const maxProcessingBytes = 10 << 20

// prepareProcessingImage bounds the image sent to OCR and the vision provider.
// Original bytes are retained separately for citations.
func prepareProcessingImage(data []byte, mimeType string) ([]byte, string, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode image header: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxProcessingPixels {
		return nil, "", fmt.Errorf("image dimensions are out of range")
	}
	if (mimeType == "image/png" || mimeType == "image/jpeg") &&
		config.Width <= maxProcessingEdge && config.Height <= maxProcessingEdge && len(data) <= maxProcessingBytes {
		return data, mimeType, nil
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	width, height := config.Width, config.Height
	if width > maxProcessingEdge || height > maxProcessingEdge {
		scale := float64(maxProcessingEdge) / float64(max(width, height))
		width = max(1, int(float64(width)*scale))
		height = max(1, int(float64(height)*scale))
	}
	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		sy := bounds.Min.Y + y*bounds.Dy()/height
		for x := 0; x < width; x++ {
			sx := bounds.Min.X + x*bounds.Dx()/width
			resized.Set(x, y, source.At(sx, sy))
		}
	}
	var output bytes.Buffer
	if mimeType == "image/jpeg" {
		if err := jpeg.Encode(&output, resized, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", err
		}
	} else {
		if err := png.Encode(&output, resized); err != nil {
			return nil, "", err
		}
		mimeType = "image/png"
	}
	if output.Len() > maxProcessingBytes {
		output.Reset()
		if err := jpeg.Encode(&output, resized, &jpeg.Options{Quality: 80}); err != nil {
			return nil, "", err
		}
		mimeType = "image/jpeg"
	}
	if output.Len() > maxProcessingBytes {
		return nil, "", fmt.Errorf("processing copy is too large")
	}
	return output.Bytes(), mimeType, nil
}
