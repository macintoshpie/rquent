package main

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"os"
)

// Get NRGBA color as hex string
func hexify(c color.NRGBA) string {
	return fmt.Sprintf("#%.2x%.2x%.2x", c.R, c.G, c.B)
}

// Download an file from a url and save to fd
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
