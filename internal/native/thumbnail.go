package native

import (
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"

	"ottr/internal/library"
	player "ottr/internal/player"
)

const thumbnailWidth = 320

// ensureThumbnails decodes each visible item once, in the background. Video
// uses its first frame; screenshots use the image itself.
func (a *App) ensureThumbnails() {
	if a.thumbs == nil {
		a.thumbs = make(map[int64]*ui.Bitmap)
	}
	if a.thumbLoading == nil {
		a.thumbLoading = make(map[int64]bool)
	}
	for _, m := range a.media {
		if m.Source != "screenshot" && !m.HasVideo {
			continue
		}
		if _, done := a.thumbs[m.ID]; done || a.thumbLoading[m.ID] {
			continue
		}
		a.thumbLoading[m.ID] = true
		go func(m library.Media) {
			var img image.Image
			if m.Source == "screenshot" {
				img = decodeStill(m.Path)
			} else {
				img, _ = decodeVideoPoster(m)
			}
			a.update(func() {
				delete(a.thumbLoading, m.ID)
				if img != nil {
					a.thumbs[m.ID] = ui.NewBitmap(scaleThumbnail(img))
				} else {
					a.thumbs[m.ID] = nil // do not retry a missing/corrupt file every reload
				}
			})
		}(m)
	}
}

func decodeStill(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

func decodeVideoPoster(m library.Media) (image.Image, error) {
	decoder, err := player.OpenVideo(context.Background(), m.Path, m.Width, m.Height)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	frame, err := decoder.Next()
	if err != nil {
		return nil, err
	}
	return frame.Image, nil
}

func scaleThumbnail(src image.Image) image.Image {
	bounds := src.Bounds()
	if bounds.Dx() <= thumbnailWidth {
		return src
	}
	height := bounds.Dy() * thumbnailWidth / bounds.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, thumbnailWidth, height))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}
