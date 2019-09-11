package main

import (
	"bufio"
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getJobChn(jobChn <-chan RqJob) (RqJob, error) {
	select {
	case job := <-jobChn:
		return job, nil
	default:
		return RqJob{}, errors.New("No job in channel")
	}
}

func getErrorChn(errorChn <-chan RqError) (RqError, error) {
	select {
	case rqError := <-errorChn:
		return rqError, nil
	default:
		return RqError{}, errors.New("No error in channel")
	}
}

var testPipeConfig = PipeConfig{1, 1, 1}

func TestMakePipeline(t *testing.T) {
	s := `test.com/valid`
	imageURLs := strings.NewReader(s)
	var b bytes.Buffer
	output := bufio.NewWriter(&b)
	_, err := NewPipeline(testPipeConfig).
		WithClient(testClient).
		WithSource(imageURLs).
		WithOutput(output).
		Init()

	if err != nil {
		t.Errorf("Expected (nil) Got (%v)", err)
	}
}

// func TestPipelineReadURLs(t *testing.T) {
// 	s := []string{"web1.com", "web2.com", "web3.com", "web4.com"}
// 	imageURLs := strings.NewReader(strings.Join(s, "\n"))
// 	outChn := make(chan RqJob, 10)
// 	go readURLs(imageURLs, outChn)
// 	done := false
// 	for done == false {
// 		select {
// 		case <-time.After(10 * time.Second):
// 			t.Fatal("Expected (read from outChn) Got (timeout)")
// 		case job := <-outChn:
// 			if job.doneFlag {
// 				done = true
// 				continue
// 			}
// 			if !stringInSlice(job.image.URL, s) {
// 				t.Errorf("Expected (%v in slice) Got (not in slice)", job.image.URL)
// 			}
// 		}
// 	}
// }

func TestPipelineDownloadImageOK(t *testing.T) {
	// Test that downloadImage downloads a valid image to a local file and there are no errors
	outChn := make(chan RqJob, 10)
	defer close(outChn)
	job := RqJob{
		image:   NewRqImage(testImageURL200), // URL for a VALID image
		nextChn: outChn,
	}
	errorChn := make(chan RqError, 10)
	defer close(errorChn)
	downloadImage(job, testClient, errorChn)

	select {
	case jobOut := <-outChn:
		// verify image was downloaded
		if jobOut.image.filePath == "" {
			t.Errorf("Expected (image to have file path) Got (empty string)")
		}
		if _, err := os.Stat(jobOut.image.filePath); err != nil {
			t.Errorf("Expected (image %v to exist) Got (not exists)", jobOut.image.filePath)
		}
	default:
		t.Error("Expected (job to be in out chn) Got (out chn empty)")
	}

	select {
	case err := <-errorChn:
		t.Errorf("Expected (error chn empty) Got (%v)", err.errorMsg)
	default:
		// do nothing
	}
}

func TestPipelineDownloadImage404(t *testing.T) {
	// Test that downloading an invalid URL results in an error and does not pass it to the next chn
	outChn := make(chan RqJob, 10)
	job := RqJob{
		image:   NewRqImage(testImageURL404), // URL that results in 404
		nextChn: outChn,
	}
	errorChn := make(chan RqError, 10)
	downloadImage(job, testClient, errorChn)

	select {
	case jobOut := <-outChn:
		t.Errorf("Expected (out chn to be empty) Got (%v)", jobOut)
	default:
		// do nothing
	}

	select {
	case err := <-errorChn:
		if err.errorType != RqErrorDownload {
			t.Errorf("Expected (%v) Got (%v)", RqErrorDownload, err.errorType)
		}
	default:
		t.Error("Expected (error chn to have error) Got (empty chn)")
	}
}

func TestPipelineSummarizeImageOK(t *testing.T) {
	// Test summarizing valid image put's job in next channel, the image summary is updated,
	//   and there's nothing in the error channel
	validImage := RqImage{
		URL:      testImageURL200,
		filePath: testImagePathValid, // path to a VALID local image
	}
	outChn := make(chan RqJob, 10)
	job := RqJob{
		image:   validImage,
		nextChn: outChn,
	}

	errorChn := make(chan RqError, 10)

	summarizeImage(job, errorChn)

	jobOut, err := getJobChn(outChn)
	if err != nil {
		t.Errorf("Expected (job in chn) Got (%v)", err)
	}
	if len(jobOut.image.summary.colors) == 0 {
		t.Errorf("Expected (image to have summary) Got (image has no summary)")
	}

	errOut, err := getErrorChn(errorChn)
	if err == nil {
		t.Errorf("Expected (no RqError) Got (%v)", errOut.errorMsg)
	}
}

func TestPipelineSummarizeImageBad(t *testing.T) {
	// Test that summarizing a bad image results in no job in the next channel, and an error in the
	//   error channel
	invalidImage := RqImage{
		URL:      testImageURL200,
		filePath: testImagePathInvalid, // path to an INVALID local image
	}
	outChn := make(chan RqJob, 10)
	job := RqJob{
		image:   invalidImage,
		nextChn: outChn,
	}

	errorChn := make(chan RqError, 10)

	summarizeImage(job, errorChn)

	// there should NOT be a job in the output channel
	jobOut, err := getJobChn(outChn)
	if err == nil {
		t.Errorf("Expected (job not in chn) Got (%v)", jobOut)
	}
	if len(jobOut.image.summary.colors) != 0 {
		t.Errorf("Expected (image summary not updated) Got (image summary updated)")
	}

	// there SHOULD be an error in the errorChn
	rqErr, err := getErrorChn(errorChn)
	if err != nil {
		t.Errorf("Expected (RqError in errorChn) Got (%v)", err)
	}
	if rqErr.errorType != RqErrorSummarize {
		t.Errorf("Expected (%v) Got (%v)", RqErrorSummarize, rqErr.errorType)
	}
}

func TestPipelineCleanupImageOK(t *testing.T) {
	// Test cleanup image (in this case an empty file) put's job in next chn, the file is gone,
	//   and there are no errors
	tmpFile, err := ioutil.TempFile(".", "*.jpg")
	if err != nil {
		t.Fatal(err)
	}
	tmpFilePath := tmpFile.Name()
	tmpFile.Close()

	validImage := RqImage{
		URL:      testImageURL200,
		filePath: tmpFile.Name(), // path to a file that exists
	}
	outChn := make(chan RqJob, 10)
	job := RqJob{
		image:   validImage,
		nextChn: outChn,
	}

	errorChn := make(chan RqError, 10)

	cleanupImage(job, errorChn)

	_, err = getJobChn(outChn)
	if err != nil {
		t.Errorf("Expected (job in chn) Got (%v)", err)
	}
	if fileExists(tmpFilePath) {
		t.Errorf("Expected (%v to not exist) Got (file exists)", tmpFilePath)
	}

	errOut, err := getErrorChn(errorChn)
	if err == nil {
		t.Errorf("Expected (no RqError) Got (%v)", errOut.errorMsg)
	}
}

func TestPipelineCleanupImageNoFilePath(t *testing.T) {
	// Test cleanup image when filePath is empty: put's job in next chn, and there are no errors
	validImage := RqImage{
		URL:      testImageURL200,
		filePath: "", // path is EMPTY
	}
	outChn := make(chan RqJob, 10)
	job := RqJob{
		image:   validImage,
		nextChn: outChn,
	}

	errorChn := make(chan RqError, 10)

	cleanupImage(job, errorChn)

	_, err := getJobChn(outChn)
	if err != nil {
		t.Errorf("Expected (job in chn) Got (%v)", err)
	}

	errOut, err := getErrorChn(errorChn)
	if err == nil {
		t.Errorf("Expected (no RqError) Got (%v)", errOut.errorMsg)
	}
}

func TestPipelineCleanupImageBadPath(t *testing.T) {
	// Test cleanup image when filePath is empty: put's job in next chn, and there are no errors
	img := RqImage{
		URL:      testImageURL200,
		filePath: "bogus/path.jpg", // file does not exist
	}
	outChn := make(chan RqJob, 10)
	job := RqJob{
		image:   img,
		nextChn: outChn,
	}

	errorChn := make(chan RqError, 10)

	cleanupImage(job, errorChn)

	jobOut, err := getJobChn(outChn)
	if err == nil {
		t.Errorf("Expected (job not in chn) Got (%v)", jobOut)
	}

	_, err = getErrorChn(errorChn)
	if err != nil {
		t.Errorf("Expected (RqError in errorChn) Got (%v)", err)
	}
}

func TestPipelineRunSimpleOK(t *testing.T) {
	// Test a simple input for the pipeline
	s := testImageURL200
	imageURLs := strings.NewReader(s)
	b := new(bytes.Buffer)
	// csvOut := bufio.NewWriter(b)
	pipeline, err := NewPipeline(testPipeConfig).
		WithClient(testClient).
		WithSource(imageURLs).
		WithOutput(b).
		Init()

	if err != nil {
		t.Errorf("Expected (nil) Got (%v)", err)
	}

	pipeline.Run()
	outString := b.String()
	if len(outString) == 0 {
		t.Errorf("Expected (bytesBuffered != 0), Got (0)")
	}
}

func benchmarkPipeline(nWorkers, nImages int, b *testing.B) {
	// TODO: refactor - nWorkers is not being used
	s := strings.Repeat(testImageURL200+"\n", nImages)
	for n := 0; n < b.N; n++ {
		buff := new(bytes.Buffer)
		imageURLs := strings.NewReader(s)
		pipeline, err := NewPipeline(testPipeConfig).
			WithClient(testClient).
			WithSource(imageURLs).
			WithOutput(buff).
			Init()
		if err != nil {
			b.Fatal(err)
		}

		pipeline.Run()
	}
}

func BenchmarkPipeline_1Workers_10Images(b *testing.B) {
	benchmarkPipeline(1, 10, b)
}

func BenchmarkPipeline_3Workers_10Images(b *testing.B) {
	benchmarkPipeline(1, 10, b)
}
