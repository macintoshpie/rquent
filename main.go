package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

const USAGE = `Usage: ./rquent <image_file> <csv_out>\n
  image_file = file that contains batch of images\n
  csv_out = file to write results to`

func processLine(imageUrl string, httpClient http.Client) ([3]color.NRGBA, error) {
	// download the image
	tmpImg, err := ioutil.TempFile("", "*.jpeg")
	if err != nil {
		return [3]color.NRGBA{}, err
	}
	defer tmpImg.Close()
	defer os.Remove(tmpImg.Name())
	if err := downloadToFile(imageUrl, tmpImg, &httpClient); err != nil {
		return [3]color.NRGBA{}, err
	}

	// open the image
	fmt.Printf("Processing %v (%v)...\n", imageUrl, tmpImg.Name())
	img, _, err := image.Decode(tmpImg)
	if err != nil {
		return [3]color.NRGBA{}, err
	}

	// process the image
	return getPrevalentColors(img)
}

func main() {
	// TODO: use flags package for args
	if len(os.Args) < 3 {
		fmt.Println(USAGE)
		os.Exit(1)
	}
	// open the file with image URLs and setup the CSV output file
	imagesPath := strings.TrimSpace(os.Args[1])
	csvoutPath := strings.TrimSpace(os.Args[2])

	csvoutFile, err := os.Create(csvoutPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer csvoutFile.Close()
	csvoutWriter := csv.NewWriter(csvoutFile)

	imagesFile, err := os.Open(imagesPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer imagesFile.Close()
	scanner := bufio.NewScanner(imagesFile)

	httpClient := newClient(defaultTimeout)

	// process each line
	for scanner.Scan() {
		imgUrl := scanner.Text()
		res, err := processLine(imgUrl, *httpClient)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("    Most Frequent: %v, %v, %v\n", hexify(res[0]), hexify(res[1]), hexify(res[2]))
		// write result to file
		err = csvoutWriter.Write([]string{imgUrl, hexify(res[0]), hexify(res[1]), hexify(res[2])})
		if err != nil {
			fmt.Printf("Error writing %v: %v", imgUrl, err)
		}
	}

	csvoutWriter.Flush()
	if err := csvoutWriter.Error(); err != nil {
		fmt.Println(err)
	}
}
