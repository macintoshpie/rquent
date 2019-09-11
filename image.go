package main

import (
	"image"
	"image/color"
	"net/http"
	"time"
)

type RqImage struct {
	URL      string
	size     int
	filePath string
	summary  colorSummary
	nFails   int
}

type colorSummary struct {
	colors []color.NRGBA // most prevalent colors in sorted order (most prevalent first)
}

func NewRqImage(url string) RqImage {
	return RqImage{
		URL:      url,
		size:     -1,
		filePath: "",
		summary:  colorSummary{},
	}
}

func (img *RqImage) GetHexSummary() []string {
	hexes := make([]string, len(img.summary.colors))
	for i, c := range img.summary.colors {
		hexes[i] = hexify(c)
	}
	return hexes
}

const defaultTimeout = time.Duration(5 * time.Second)

func newClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}

// Used to indicate a color that's not from the source image; should not be modified
var PlaceholderColor = color.NRGBA{}

// Return slice of colors in sorted order of prevalence
func getPrevalentColors(imgPtr *image.Image) (colorSummary, error) {
	// TODO: generalize to k most prevalent, use a min-heap
	img := *imgPtr

	counts := make(map[color.NRGBA]uint64)
	counts[PlaceholderColor] = 0
	mostColors := []color.NRGBA{PlaceholderColor, PlaceholderColor, PlaceholderColor}

	bounds := img.Bounds()
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			// convert color at x, y to NRGBA
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			c.A = 255
			counts[c] += 1

			// TODO: consider factoring out into function
			// update most frequent colors as needed
			if c == mostColors[0] || c == mostColors[1] || c == mostColors[2] {
				// case 1: color is already one of the most frequent - check if it needs to be swapped
				for j := 1; j < 3; j += 1 {
					if c == mostColors[j] && counts[c] > counts[mostColors[j-1]] {
						mostColors[j-1], mostColors[j] = mostColors[j], mostColors[j-1]
						break
					}
				}
			} else {
				// case 2: color is not one of the most frequent - insert at first empty slot or the end
				if counts[c] > counts[mostColors[2]] {
					for i := 0; i < 3; i += 1 {
						if mostColors[i] == PlaceholderColor {
							mostColors[i] = c
							break
						} else if i == 2 {
							mostColors[2] = c
						}
					}
				}
			}
		}
	}

	return colorSummary{mostColors}, nil
}
