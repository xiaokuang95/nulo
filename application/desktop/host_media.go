package main

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaokuang95/nulo/sdk"
)

const maxImageAttachmentBytes = 10 << 20

type hostAttachment struct {
	Path  string
	Image *sdk.ImageAttachment
}

func loadHostAttachments(paths []string) ([]hostAttachment, error) {
	out := make([]hostAttachment, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		item, err := loadHostAttachment(path)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func loadHostAttachment(path string) (hostAttachment, error) {
	info, err := os.Stat(path)
	if err != nil {
		return hostAttachment{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if info.IsDir() {
		return hostAttachment{}, fmt.Errorf("%s is a directory", filepath.Base(path))
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if !strings.HasPrefix(mimeType, "image/") {
		head := make([]byte, 512)
		file, err := os.Open(path)
		if err != nil {
			return hostAttachment{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		n, _ := file.Read(head)
		_ = file.Close()
		mimeType = http.DetectContentType(head[:n])
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return hostAttachment{Path: path}, nil
	}
	if info.Size() > maxImageAttachmentBytes {
		return hostAttachment{}, fmt.Errorf("%s is larger than %d MB", filepath.Base(path), maxImageAttachmentBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return hostAttachment{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if len(data) == 0 {
		return hostAttachment{}, fmt.Errorf("%s is empty", filepath.Base(path))
	}
	return hostAttachment{
		Path:  path,
		Image: &sdk.ImageAttachment{Data: base64.StdEncoding.EncodeToString(data), MIMEType: mimeType},
	}, nil
}

func isImagePath(path string) bool {
	return strings.HasPrefix(mime.TypeByExtension(strings.ToLower(filepath.Ext(path))), "image/")
}
