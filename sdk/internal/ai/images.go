package ai

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"math"
)

// Image resize defaults from Pi's models.md: 2000x2000, a 4.5 MiB encoded
// payload and JPEG quality 80.
const (
	defaultImageMaxWidth  = 2000
	defaultImageMaxHeight = 2000
	defaultImageMaxBytes  = 4718592
	defaultImageQuality   = 80
)

// withResizedImages rewrites the transcript's images to the model's resize
// profile. Images that already fit are returned untouched, so sending the same
// history twice does not degrade it.
func withResizedImages(tr Transcript, limits *InputLimits) Transcript {
	if limits == nil || limits.Images == nil || limits.Images.Resize == nil {
		return tr
	}
	resize := *limits.Images.Resize
	if resize.MaxWidth <= 0 {
		resize.MaxWidth = defaultImageMaxWidth
	}
	if resize.MaxHeight <= 0 {
		resize.MaxHeight = defaultImageMaxHeight
	}
	if resize.MaxBytes <= 0 {
		resize.MaxBytes = defaultImageMaxBytes
	}
	if resize.JPEGQuality <= 0 || resize.JPEGQuality > 100 {
		resize.JPEGQuality = defaultImageQuality
	}
	messages := tr.Messages
	copied := false
	for i, message := range messages {
		blocks := message.ContentBlocks()
		changed := false
		for j, block := range blocks {
			if block.Type != "image" {
				continue
			}
			resized := resizeImageBlock(block, resize)
			if resized.Data != block.Data {
				blocks[j] = resized
				changed = true
			}
		}
		if !changed {
			continue
		}
		if !copied {
			messages = append([]Message(nil), messages...)
			copied = true
		}
		messages[i].Content = EncodeContentBlocks(blocks)
	}
	if !copied {
		return tr
	}
	return Transcript{Messages: messages}
}

// resizeImageBlock scales and re-encodes one base64 image. Anything that cannot
// be decoded (WebP, for one) is returned unchanged rather than dropped.
func resizeImageBlock(block ContentBlock, options ImageResizeOptions) ContentBlock {
	raw, err := base64.StdEncoding.DecodeString(block.Data)
	if err != nil || len(raw) == 0 {
		return block
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return block
	}
	// An animated GIF loses its frames when decoded, so leave it alone.
	if format == "gif" {
		return block
	}
	if config.Width <= options.MaxWidth && config.Height <= options.MaxHeight && len(block.Data) <= options.MaxBytes {
		return block
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return block
	}
	img = scaleDown(img, options.MaxWidth, options.MaxHeight)

	quality := options.JPEGQuality
	for attempt := 0; attempt < 12; attempt++ {
		encoded, mime := encodeImage(img, format, quality)
		if len(encoded) > 0 && base64Len(encoded) <= options.MaxBytes {
			return ContentBlock{Type: "image", Data: base64.StdEncoding.EncodeToString(encoded), MIMEType: mime}
		}
		if quality > 40 {
			quality -= 10
			continue
		}
		bounds := img.Bounds()
		width := max(1, int(math.Floor(float64(bounds.Dx())*0.8)))
		height := max(1, int(math.Floor(float64(bounds.Dy())*0.8)))
		img = scaleDown(img, width, height)
	}
	return block
}

// scaleDown fits the image inside maxWidth by maxHeight, keeping its aspect
// ratio. It never enlarges.
func scaleDown(img image.Image, maxWidth, maxHeight int) image.Image {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxWidth && height <= maxHeight {
		return img
	}
	scale := math.Min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
	targetWidth := max(1, int(math.Round(float64(width)*scale)))
	targetHeight := max(1, int(math.Round(float64(height)*scale)))
	return resizeNearest(img, targetWidth, targetHeight)
}

// resizeNearest is a small dependency-free scaler; providers bill by pixels
// rather than fidelity, and the images are screenshots and photos.
func resizeNearest(src image.Image, width, height int) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + y*bounds.Dy()/height
		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + x*bounds.Dx()/width
			dst.Set(x, y, src.At(sourceX, sourceY))
		}
	}
	return dst
}

// encodeImage keeps PNG (for transparency) and encodes everything else as JPEG.
func encodeImage(img image.Image, format string, quality int) ([]byte, string) {
	var buffer bytes.Buffer
	if format == "png" {
		if err := png.Encode(&buffer, img); err == nil {
			return buffer.Bytes(), "image/png"
		}
		buffer.Reset()
	}
	if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: quality}); err == nil {
		return buffer.Bytes(), "image/jpeg"
	}
	return nil, ""
}

func base64Len(data []byte) int {
	return base64.StdEncoding.EncodedLen(len(data))
}
