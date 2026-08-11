//go:build !windows

package capture

import (
	"fmt"
	"image"
	"image/color"
)

func ScreenSize() (int, int) {
	return 1280, 720
}

func PrimaryScreen() (image.Image, error) {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			img.Set(x, y, color.RGBA{R: 20, G: 30, B: 50, A: 255})
		}
	}
	return img, fmt.Errorf("screen capture is Windows-only in this MVP build")
}
