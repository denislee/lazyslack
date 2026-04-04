package component

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

var avatarHTTPClient = &http.Client{Timeout: 10 * time.Second}

var avatarCache sync.Map

// FetchAndRenderAvatar fetches an image URL and renders it as half-block Unicode art.
// width is the number of terminal columns and pixels. Output is width columns × width rows.
func FetchAndRenderAvatar(url string, width int) (string, error) {
	if url == "" {
		return "", nil
	}
	cacheKey := fmt.Sprintf("%s:%d", url, width)
	if cached, ok := avatarCache.Load(cacheKey); ok {
		return cached.(string), nil
	}

	img, err := fetchImage(url)
	if err != nil {
		return "", err
	}

	// Resize to width x width pixels (square source).
	// Half-block halves the height, and terminal cells are ~2:1, so this looks square.
	pixelH := width*2/3 + 2
	resized := resizeImage(img, width, pixelH)
	rendered := renderHalfBlock(resized)

	avatarCache.Store(cacheKey, rendered)
	return rendered, nil
}

func fetchImage(url string) (image.Image, error) {
	resp, err := avatarHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("avatar fetch: HTTP %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
	return img, err
}

func resizeImage(img image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

func renderHalfBlock(img image.Image) string {
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y

	var sb strings.Builder
	for y := bounds.Min.Y; y < bounds.Min.Y+h; y += 2 {
		if y > bounds.Min.Y {
			sb.WriteString("\n")
		}
		for x := bounds.Min.X; x < bounds.Min.X+w; x++ {
			// Top pixel -> foreground, bottom pixel -> background
			tr, tg, tb, _ := img.At(x, y).RGBA()
			br, bg, bb := uint32(0), uint32(0), uint32(0)
			if y+1 < bounds.Min.Y+h {
				br, bg, bb, _ = img.At(x, y+1).RGBA()
			}
			// RGBA returns 16-bit values, shift to 8-bit
			fmt.Fprintf(&sb, "\033[38;2;%d;%d;%dm\033[48;2;%d;%d;%dm\u2580",
				tr>>8, tg>>8, tb>>8,
				br>>8, bg>>8, bb>>8,
			)
		}
		sb.WriteString("\033[0m")
	}
	return sb.String()
}
