package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type imagePoint struct {
	X float32
	Y float32
}

type imageStroke []imagePoint

type cachedImagePreview struct {
	bitmap   *ui.Bitmap
	modified int64
	size     int64
	failed   bool
}

func (a *app) attachmentBitmap(path string) *ui.Bitmap {
	if path == "" || !isImagePath(path) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil
	}
	if a.imagePreviews == nil {
		a.imagePreviews = make(map[string]cachedImagePreview)
	}
	if cached, ok := a.imagePreviews[path]; ok && cached.modified == info.ModTime().UnixNano() && cached.size == info.Size() {
		return cached.bitmap
	}
	entry := cachedImagePreview{modified: info.ModTime().UnixNano(), size: info.Size()}
	if info.Size() > maxImageAttachmentBytes {
		entry.failed = true
		a.imagePreviews[path] = entry
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		entry.failed = true
		a.imagePreviews[path] = entry
		return nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width*config.Height > 24_000_000 {
		entry.failed = true
		a.imagePreviews[path] = entry
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		entry.failed = true
		a.imagePreviews[path] = entry
		return nil
	}
	if config.Width > 512 || config.Height > 512 {
		scale := float64(512) / float64(max(config.Width, config.Height))
		width := max(1, int(float64(config.Width)*scale))
		height := max(1, int(float64(config.Height)*scale))
		scaled := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Over, nil)
		img = scaled
	}
	entry.bitmap = ui.NewBitmap(img)
	a.imagePreviews[path] = entry
	return entry.bitmap
}

func imageDimensions(path string) (int, int, bool) {
	if path == "" {
		return 0, 0, false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxImageAttachmentBytes {
		return 0, 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, false
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return 0, 0, false
	}
	return config.Width, config.Height, true
}

func loadImageFile(path string) (image.Image, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() || info.Size() > maxImageAttachmentBytes {
		return nil, fmt.Errorf("%s is not a supported image", filepath.Base(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func (a *app) imageThumbButton(c *ui.Context, path string, size float32) bool {
	bitmap := a.attachmentBitmap(path)
	if bitmap == nil {
		return false
	}
	button := ui.ButtonBase(c).Size(size, size).Radius(4).Label("Preview " + filepath.Base(path))
	button.Children(func() {
		ui.Image(c, bitmap).Size(size, size).Fit(ui.Cover).Radius(4)
	})
	if button.Clicked() {
		a.openImagePreview(path)
	}
	return true
}

func (a *app) pasteClipboardImage(c *ui.Context, text string) bool {
	if c == nil || text != "" {
		return false
	}
	data := c.Services().ReadClipboardData(transfer.PNG)
	items := data.Items()
	if len(items) == 0 {
		return false
	}
	dir := clipboardImageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		a.pendingToast = "Paste image: " + err.Error()
		return true
	}
	var added []string
	for i, item := range items {
		raw, err := item.Read(transfer.PNG)
		if err != nil || len(raw) == 0 {
			continue
		}
		if len(raw) > maxImageAttachmentBytes {
			a.pendingToast = fmt.Sprintf("Pasted image is larger than %d MB", maxImageAttachmentBytes>>20)
			return true
		}
		name := fmt.Sprintf("clipboard-%d-%d.png", time.Now().UnixNano(), i)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			a.pendingToast = "Paste image: " + err.Error()
			return true
		}
		added = append(added, path)
	}
	if len(added) == 0 {
		return false
	}
	a.addAttachments(added)
	if len(added) == 1 {
		a.pendingToast = "Pasted image"
	} else {
		a.pendingToast = fmt.Sprintf("Pasted %d images", len(added))
	}
	return true
}

func (a *app) openImagePreview(path string) {
	a.imagePath = path
	a.imageOpen = true
	a.cropMode = false
	a.cropDragging = false
	a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
	a.imageRotation = 0
	a.imageFlipX = false
	a.imageFlipY = false
	a.imageEditBitmap = nil
	a.imageAnnotate = false
	a.imageAnnotDrawing = false
	a.imageStrokes = nil
	a.invalidateAnnotations()
}

func (a *app) resetImageEdit() {
	a.cropDragging = false
	a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
	a.imageRotation = 0
	a.imageFlipX = false
	a.imageFlipY = false
	a.imageEditBitmap = nil
	a.imageAnnotate = false
	a.imageAnnotDrawing = false
	a.imageStrokes = nil
	a.invalidateAnnotations()
}

func (a *app) invalidateAnnotations() {
	a.imageAnnotVersion++
	a.imageAnnotBitmap = nil
	a.imageAnnotCachedVer = -1
}

func transformImage(img image.Image, rotation int, flipX, flipY bool) image.Image {
	rotation = ((rotation % 360) + 360) % 360
	if rotation == 0 && !flipX && !flipY {
		return img
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	outW, outH := width, height
	if rotation == 90 || rotation == 270 {
		outW, outH = height, width
	}
	dst := image.NewNRGBA(image.Rect(0, 0, outW, outH))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			fx, fy := x, y
			if flipX {
				fx = width - 1 - fx
			}
			if flipY {
				fy = height - 1 - fy
			}
			dx, dy := fx, fy
			switch rotation {
			case 90:
				dx, dy = height-1-fy, fx
			case 180:
				dx, dy = width-1-fx, height-1-fy
			case 270:
				dx, dy = fy, width-1-fx
			}
			dst.Set(dx, dy, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}

func (a *app) transformedPreviewBitmap() *ui.Bitmap {
	img, err := loadImageFile(a.imagePath)
	if err != nil {
		return nil
	}
	img = transformImage(img, a.imageRotation, a.imageFlipX, a.imageFlipY)
	img = drawStrokesOnImage(img, a.imageStrokes)
	bounds := img.Bounds()
	maxW, maxH := 1200, 900
	if bounds.Dx() > maxW || bounds.Dy() > maxH {
		scale := min(float64(maxW)/float64(bounds.Dx()), float64(maxH)/float64(bounds.Dy()))
		width := max(1, int(float64(bounds.Dx())*scale))
		height := max(1, int(float64(bounds.Dy())*scale))
		scaled := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Over, nil)
		img = scaled
	}
	return ui.NewBitmap(img)
}

func (a *app) saveCroppedImage() error {
	img, err := loadImageFile(a.imagePath)
	if err != nil {
		return err
	}
	img = transformImage(img, a.imageRotation, a.imageFlipX, a.imageFlipY)
	img = drawStrokesOnImage(img, a.imageStrokes)
	bounds := img.Bounds()
	x0, x1 := min(a.cropStartX, a.cropEndX), max(a.cropStartX, a.cropEndX)
	y0, y1 := min(a.cropStartY, a.cropEndY), max(a.cropStartY, a.cropEndY)
	x0, y0 = clamp01(x0), clamp01(y0)
	x1, y1 = clamp01(x1), clamp01(y1)
	rect := image.Rect(
		bounds.Min.X+int(math.Round(float64(x0)*float64(bounds.Dx()))),
		bounds.Min.Y+int(math.Round(float64(y0)*float64(bounds.Dy()))),
		bounds.Min.X+int(math.Round(float64(x1)*float64(bounds.Dx()))),
		bounds.Min.Y+int(math.Round(float64(y1)*float64(bounds.Dy()))),
	).Intersect(bounds)
	if rect.Dx() < 2 || rect.Dy() < 2 {
		return fmt.Errorf("crop selection is too small")
	}
	dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
	dir := clipboardImageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	encoded, ext, err := encodeCroppedImage(dst)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("crop-%d%s", time.Now().UnixNano(), ext))
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	replaced := false
	for i, existing := range a.attachments {
		if existing == a.imagePath {
			a.attachments[i] = path
			replaced = true
			break
		}
	}
	if !replaced {
		a.attachments = append(a.attachments, path)
	}
	a.imageOpen = false
	a.cropMode = false
	a.resetImageEdit()
	a.pendingToast = "Edited image attached"
	return nil
}

func annotationRadius(width, height int) int {
	return max(2, int(float64(min(width, height))*0.007))
}

func drawBrush(dst *image.NRGBA, x, y, radius int) {
	bounds := dst.Bounds()
	for py := y - radius; py <= y+radius; py++ {
		for px := x - radius; px <= x+radius; px++ {
			if px < bounds.Min.X || py < bounds.Min.Y || px >= bounds.Max.X || py >= bounds.Max.Y {
				continue
			}
			dx, dy := px-x, py-y
			if dx*dx+dy*dy <= radius*radius {
				dst.SetNRGBA(px, py, color.NRGBA{R: 255, G: 91, B: 91, A: 255})
			}
		}
	}
}

func drawStrokeSegment(dst *image.NRGBA, from, to imagePoint, radius int) {
	width, height := dst.Bounds().Dx(), dst.Bounds().Dy()
	x0, y0 := int(from.X*float32(width)), int(from.Y*float32(height))
	x1, y1 := int(to.X*float32(width)), int(to.Y*float32(height))
	dx, dy := x1-x0, y1-y0
	steps := max(absInt(dx), absInt(dy))
	if steps == 0 {
		drawBrush(dst, x0, y0, radius)
		return
	}
	for i := 0; i <= steps; i++ {
		x := x0 + dx*i/steps
		y := y0 + dy*i/steps
		drawBrush(dst, x, y, radius)
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func renderStrokes(width, height int, strokes []imageStroke) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	radius := annotationRadius(width, height)
	for _, stroke := range strokes {
		if len(stroke) == 0 {
			continue
		}
		if len(stroke) == 1 {
			drawBrush(dst, int(stroke[0].X*float32(width)), int(stroke[0].Y*float32(height)), radius)
			continue
		}
		for i := 1; i < len(stroke); i++ {
			drawStrokeSegment(dst, stroke[i-1], stroke[i], radius)
		}
	}
	return dst
}

func drawStrokesOnImage(img image.Image, strokes []imageStroke) image.Image {
	if len(strokes) == 0 {
		return img
	}
	bounds := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
	marked := renderStrokes(bounds.Dx(), bounds.Dy(), strokes)
	draw.Draw(dst, dst.Bounds(), marked, image.Point{}, draw.Over)
	return dst
}

func (a *app) annotationOverlay(width, height int) *ui.Bitmap {
	if len(a.imageStrokes) == 0 && !a.imageAnnotate {
		return nil
	}
	w, h := max(1, int(width)), max(1, int(height))
	if a.imageAnnotBitmap != nil && a.imageAnnotCachedVer == a.imageAnnotVersion && a.imageAnnotW == w && a.imageAnnotH == h {
		return a.imageAnnotBitmap
	}
	a.imageAnnotBitmap = ui.NewBitmap(renderStrokes(w, h, a.imageStrokes))
	a.imageAnnotCachedVer = a.imageAnnotVersion
	a.imageAnnotW, a.imageAnnotH = w, h
	return a.imageAnnotBitmap
}

func encodeCroppedImage(img image.Image) ([]byte, string, error) {
	bounds := img.Bounds()
	pixels := bounds.Dx() * bounds.Dy()
	if pixels <= 4_000_000 {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err == nil && buf.Len() <= maxImageAttachmentBytes {
			return buf.Bytes(), ".png", nil
		}
	}
	var buf bytes.Buffer
	for _, quality := range []int{90, 80, 70, 60} {
		buf.Reset()
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err == nil && buf.Len() <= maxImageAttachmentBytes {
			return buf.Bytes(), ".jpg", nil
		}
	}
	current := img
	for scale := 0.8; bounds.Dx() > 32 && bounds.Dy() > 32; scale *= 0.8 {
		bounds = current.Bounds()
		nextW := max(1, int(float64(bounds.Dx())*scale))
		nextH := max(1, int(float64(bounds.Dy())*scale))
		if nextW == bounds.Dx() && nextH == bounds.Dy() {
			break
		}
		next := image.NewNRGBA(image.Rect(0, 0, nextW, nextH))
		draw.CatmullRom.Scale(next, next.Bounds(), current, bounds, draw.Src, nil)
		current = next
		buf.Reset()
		if err := jpeg.Encode(&buf, current, &jpeg.Options{Quality: 80}); err == nil && buf.Len() <= maxImageAttachmentBytes {
			return buf.Bytes(), ".jpg", nil
		}
	}
	return nil, "", fmt.Errorf("cropped image is still larger than %d MB", maxImageAttachmentBytes/(1024*1024))
}

func clamp01(value float32) float32 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clipboardImageDir() string {
	cache, err := os.UserCacheDir()
	if err != nil || cache == "" {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "nulo", "clipboard")
}
