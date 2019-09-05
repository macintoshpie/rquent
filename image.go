package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"net/http"
	"os"
	"time"
)

const defaultTimeout = time.Duration(5 * time.Second)

func newClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}

// Download an image from a url and save to fd
func downloadToFile(url string, localFile *os.File, client *http.Client) error {
	// Ref: https://golangcode.com/download-a-file-from-a-url/
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return errors.New(fmt.Sprintf("Url invalid (statusCode %v", resp.StatusCode))
	}

	_, err = io.Copy(localFile, resp.Body)
	if err != nil {
		return err
	}

	_, err = localFile.Seek(0, 0)
	return err
}

// Get NRGBA color as hex string
func hexify(c color.NRGBA) string {
	return fmt.Sprintf("#%.2x%.2x%.2x", c.R, c.G, c.B)
}

// Used to indicate a color that's not from the source image
var PlaceholderColor = color.NRGBA{}

// Return slice of colors in sorted order of prevalence
func getPrevalentColors(img image.Image) ([3]color.NRGBA, error) {
	// NOTE: to generalize to k most prevalent, use a min-heap
	counts := make(map[color.NRGBA]uint64)
	counts[PlaceholderColor] = 0
	mostColors := [3]color.NRGBA{PlaceholderColor, PlaceholderColor, PlaceholderColor}

	// convert image to NRGBA pixels
	rect := img.Bounds()
	imgNRGBA := image.NewNRGBA(rect)
	draw.Draw(imgNRGBA, rect, img, rect.Min, draw.Src)
	imgPix := imgNRGBA.Pix
	imgStride := imgNRGBA.Stride

	for x := rect.Min.X; x < rect.Max.X; x++ {
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			// convert color at x, y to NRGBA
			pixel := (y-rect.Min.Y)*imgStride + (x-rect.Min.X)*4
			c := color.NRGBA{imgPix[pixel], imgPix[pixel+1], imgPix[pixel+2], 255}
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

	return mostColors, nil
}
