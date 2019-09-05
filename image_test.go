package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"io/ioutil"
	"math"
	"os"
	"testing"
)

func TestDownloadImageToFileSuccess(t *testing.T) {
	// setup
	localFile, err := ioutil.TempFile("", "*.jpg")
	if err != nil {
		t.Errorf("Failed to create tmp image")
	}
	defer localFile.Close()
	defer os.Remove(localFile.Name())

	// download the image
	imgUrl := "http://i.imgur.com/FApqk3D.jpg"
	err = downloadImageToFile(imgUrl, localFile)
	if err != nil {
		t.Errorf("Expected (nil) Got (%v)", err)
	}

	// check the file exists
	if _, err := os.Stat(localFile.Name()); err != nil {
		t.Errorf("Expected (image file to exist) Got (not exists)")
	}
}

func TestDownloadImageToFileTimeout(t *testing.T) {
	// setup
	localFile, err := ioutil.TempFile("", "*.jpg")
	if err != nil {
		t.Errorf("Failed to create tmp image")
	}
	defer localFile.Close()
	defer os.Remove(localFile.Name())

	// visit url that waits longer than our client's timeout
	imgUrl := "https://httpstat.us/200?sleep=10000"
	err = downloadImageToFile(imgUrl, localFile)
	if err == nil {
		t.Errorf("Expected (client timeout error) Got (%v)", err)
	}
}

type colorFreq struct {
	color color.NRGBA
	freq  float32
}

// Create an image with columns of single colors
// It's user's responsibility to ensure the frequencies add to 1, else the result is unpredictable
// Save image for debugging purposes
func newColorsImage(width, height int, colors []colorFreq, save bool) image.Image {
	img := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{width, height}})
	var xStart, xEnd int
	// for each color, calculate the start and end x positions, then fill in the column
	for i, c := range colors {
		if i == 0 {
			xStart = 0
		} else {
			xStart = xEnd
		}
		xEnd = xStart + int(c.freq*float32(width))

		for x := xStart; x < xEnd; x++ {
			for y := 0; y < height; y++ {
				img.Set(x, y, c.color)
			}
		}
	}

	if save {
		out, _ := os.Create("./newColorsImage.png")
		jpeg.Encode(out, img, nil)
	}

	return img
}

var red = color.NRGBA{255, 0, 0, 255}
var green = color.NRGBA{0, 255, 0, 255}
var blue = color.NRGBA{0, 0, 255, 255}
var white = color.NRGBA{255, 255, 255, 255}

var rgbSingleColorTests = []struct {
	name   string
	colors []colorFreq
}{
	{"red", []colorFreq{colorFreq{red, 1}}},
	{"green", []colorFreq{colorFreq{green, 1}}},
	{"blue", []colorFreq{colorFreq{blue, 1}}},
}

func TestGetPrevalentColorsSingleColor(t *testing.T) {
	const width, height = 10, 10
	for _, tt := range rgbSingleColorTests {
		t.Run(tt.name, func(t *testing.T) {
			colorImg := newColorsImage(width, height, tt.colors, false)
			colors, err := getPrevalentColors(colorImg)

			if err != nil {
				t.Errorf("Expected (nil) Got (%v)", err)
			}

			if colors[0] != tt.colors[0].color {
				t.Errorf("Expected (colors[0] == %v) Got (%v)", tt.colors[0].color, colors)
			}
		})
	}
}

var rgbManyColorTests = []struct {
	name         string
	colorsSorted []colorFreq
}{
	{"3 colors", []colorFreq{colorFreq{red, .5}, colorFreq{green, .3}, colorFreq{blue, .2}}},
	{"4 colors", []colorFreq{colorFreq{red, .5}, colorFreq{green, .3}, colorFreq{blue, .18}, colorFreq{white, .02}}},
	{"2 colors", []colorFreq{colorFreq{blue, .8}, colorFreq{red, .2}}},
}

func TestGetPrevalentColorsManyColors(t *testing.T) {
	const width, height = 100, 10
	for _, tt := range rgbManyColorTests {
		t.Run(tt.name, func(t *testing.T) {
			colorImg := newColorsImage(width, height, tt.colorsSorted, false)
			colors, err := getPrevalentColors(colorImg)

			if err != nil {
				t.Errorf("Expected (nil) Got (%v)", err)
			}

			// verify result
			nExpected := int(math.Min(float64(len(tt.colorsSorted)), 3))
			for i := 0; i < nExpected; i++ {
				expected := tt.colorsSorted[i].color
				if colors[i] != expected {
					t.Errorf("Expected (colors[%v] == %v) Got (%v)", i, expected, colors[i])
				}
			}

			// verify any remaining slots of results are empty (when there are less than 3 colors in image)
			if nExpected < 3 {
				for i := nExpected; i < 3; i += 1 {
					if colors[i] != PlaceholderColor {
						t.Errorf("Expected(colors[%v] == placeholder) Got (%v)", i, colors[i])
					}
				}
			}
		})
	}
}

// prevent compiler from removing result in benchmarks
var result [3]color.NRGBA

func benchmarkGetPrevalentColors(width, height int, b *testing.B) {
	var colors [3]color.NRGBA
	colorImg := newColorsImage(width, height, []colorFreq{colorFreq{red, 1}}, false)
	for n := 0; n < b.N; n++ {
		colors, _ = getPrevalentColors(colorImg)
	}

	result = colors
}

func BenchmarkGetPrevalentColors100px(b *testing.B) {
	benchmarkGetPrevalentColors(10, 10, b)
}

func BenchmarkGetPrevalentColors100_000px(b *testing.B) {
	benchmarkGetPrevalentColors(100, 100, b)
}

func BenchmarkGetPrevalentColors1_000_000px(b *testing.B) {
	benchmarkGetPrevalentColors(1000, 1000, b)
}
