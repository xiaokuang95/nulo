package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPasteImageAttachment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a, tt := newTester()
	if err := tt.Click("Message nulo"); err != nil {
		t.Fatal(err)
	}
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, testPNG(t)))))
	tt.Key(ui.Cmd, ui.KeyV)
	if len(a.attachments) != 1 {
		t.Fatalf("attachments = %v", a.attachments)
	}
	path := a.attachments[0]
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("path = %q", path)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("pasted file = %v, %v", info, err)
	}
	if bitmap := a.attachmentBitmap(path); bitmap == nil {
		t.Fatal("image preview did not decode")
	}
	if !tt.HasText("Pasted image") {
		t.Fatalf("toast missing: %v", tt.Texts())
	}
	if err := tt.Click("Preview " + filepath.Base(path)); err != nil {
		t.Fatal(err)
	}
	if !a.imageOpen {
		t.Fatal("image preview did not open")
	}
	if err := tt.Click("Crop"); err != nil {
		t.Fatal(err)
	}
	area, ok := tt.Find("Crop area")
	if !ok {
		t.Fatal("crop area not found")
	}
	tt.Press(area.X+area.W*0.1, area.Y+area.H*0.1)
	tt.Move(area.X+area.W*0.6, area.Y+area.H*0.6)
	tt.Release(area.X+area.W*0.6, area.Y+area.H*0.6)
	if a.cropStartX < 0.05 || a.cropStartX > 0.2 || a.cropEndX < 0.55 || a.cropEndX > 0.7 {
		t.Fatalf("crop x = %v..%v", a.cropStartX, a.cropEndX)
	}
	if a.cropStartY < 0.05 || a.cropStartY > 0.2 || a.cropEndY < 0.55 || a.cropEndY > 0.7 {
		t.Fatalf("crop y = %v..%v", a.cropStartY, a.cropEndY)
	}
	if err := tt.Click("Annotate"); err != nil {
		t.Fatal(err)
	}
	if !a.imageAnnotate {
		t.Fatal("annotation mode did not enable")
	}
	area, ok = tt.Find("Crop area")
	if !ok {
		t.Fatal("annotation area not found")
	}
	tt.Press(area.X+area.W*0.2, area.Y+area.H*0.2)
	tt.Move(area.X+area.W*0.8, area.Y+area.H*0.8)
	tt.Release(area.X+area.W*0.8, area.Y+area.H*0.8)
	if len(a.imageStrokes) == 0 || len(a.imageStrokes[0]) < 2 {
		t.Fatalf("annotation strokes = %+v", a.imageStrokes)
	}
	if a.annotationOverlay(80, 60) == nil {
		t.Fatal("annotation overlay was not rendered")
	}
	if err := tt.Click("Clear marks"); err != nil {
		t.Fatal(err)
	}
	if len(a.imageStrokes) != 0 {
		t.Fatalf("annotation strokes after clear = %+v", a.imageStrokes)
	}
	tt.Key(0, ui.KeyEscape)
	if a.imageOpen {
		t.Fatal("image preview did not close")
	}
}

func TestSaveCroppedImageReplacesComposerAttachment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "source.png")
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 60), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	a := baseApp()
	a.imagePath = path
	a.attachments = []string{path}
	a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0.25, 0, 0.75, 1
	if err := a.saveCroppedImage(); err != nil {
		t.Fatal(err)
	}
	if len(a.attachments) != 1 || a.attachments[0] == path {
		t.Fatalf("attachments = %v", a.attachments)
	}
	cropped := a.attachments[0]
	if !strings.HasSuffix(cropped, ".png") {
		t.Fatalf("cropped path = %q", cropped)
	}
	data, err := os.ReadFile(cropped)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 4 || config.Height != 4 {
		t.Fatalf("cropped size = %dx%d, want 4x4", config.Width, config.Height)
	}
}

func TestTransformImageOrientation(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, A: 255})
	rotated := transformImage(img, 90, false, false)
	if rotated.Bounds().Dx() != 3 || rotated.Bounds().Dy() != 2 {
		t.Fatalf("rotated bounds = %v", rotated.Bounds())
	}
	want := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	want.Set(2, 0, color.RGBA{R: 255, A: 255})
	want.Set(2, 1, color.RGBA{G: 255, A: 255})
	want.Set(1, 0, color.RGBA{B: 255, A: 255})
	want.Set(1, 1, color.RGBA{R: 255, G: 255, A: 255})
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if got, expect := color.NRGBAModel.Convert(rotated.At(x, y)), color.NRGBAModel.Convert(want.At(x, y)); got != expect {
				t.Fatalf("pixel %d,%d = %v, want %v", x, y, got, expect)
			}
		}
	}
	flipped := transformImage(img, 0, true, false)
	if got, expect := color.NRGBAModel.Convert(flipped.At(0, 0)), color.NRGBAModel.Convert(img.At(1, 0)); got != expect {
		t.Fatalf("flip pixel = %v, want %v", got, expect)
	}
}

func TestSaveCroppedImageAppliesRotation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "source.png")
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 60), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	a := baseApp()
	a.imagePath = path
	a.attachments = []string{path}
	a.imageRotation = 90
	a.imageStrokes = []imageStroke{{{X: 0.1, Y: 0.5}, {X: 0.9, Y: 0.5}}}
	a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
	if err := a.saveCroppedImage(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a.attachments[0])
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 4 || config.Height != 8 {
		t.Fatalf("rotated crop size = %dx%d, want 4x8", config.Width, config.Height)
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	pixel := color.NRGBAModel.Convert(decoded.At(2, 4)).(color.NRGBA)
	if pixel.R < 200 || pixel.G > 120 {
		t.Fatalf("annotation missing from saved crop: %+v", pixel)
	}
}

func TestDrawStrokesOnImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	out := drawStrokesOnImage(img, []imageStroke{{
		{X: 0.2, Y: 0.2}, {X: 0.8, Y: 0.8},
	}})
	pixel := color.NRGBAModel.Convert(out.At(20, 20)).(color.NRGBA)
	if pixel.R < 200 || pixel.G > 120 || pixel.B > 120 {
		t.Fatalf("annotation pixel = %+v", pixel)
	}
	plain := color.NRGBAModel.Convert(out.At(2, 2)).(color.NRGBA)
	if plain.R != 0 || plain.G != 0 || plain.B != 0 {
		t.Fatalf("plain pixel = %+v", plain)
	}
}
