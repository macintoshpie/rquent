package main

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

const USAGE = `Usage: ./rquent <image_file> <csv_out>\n
  image_file = file that contains batch of images\n
	csv_out = file to write results to`

type RqPipeline struct {
	pool         *RqPool
	sourceURLs   io.Reader
	outFile      io.Writer
	mux          sync.Mutex
	imageCount   uint64
	readURLsDone bool
}

type RqPool struct {
	nWorkers     int
	wg           sync.WaitGroup
	downloadChn  chan RqJob
	summarizeChn chan RqJob
	saveChn      chan RqJob
	cleanupChn   chan RqJob
	errorChn     chan RqError
	doneChn      chan int
	client       *http.Client
	stopOnce     sync.Once
}

type RqJob struct {
	image    RqImage
	retryChn chan RqJob
	nextChn  chan RqJob
	nFails   int
	doneFlag bool
}

type RqQueue struct {
	chn chan RqJob
	cnt uint32
}

type RqError struct {
	job       RqJob
	errorType RqErrorType
	errorMsg  string
}

type RqErrorType float64

const (
	RqErrorDownload = iota
	RqErrorSummarize
	RqErrorSave
	RqErrorCleanup
	RqErrorNoRetry
)

const RqJobMaxFails = 3

func (q *RqQueue) enqueue(job RqJob) {
	atomic.AddUint32(&q.cnt, 1)
	q.chn <- job
}

func NewRqError(job RqJob, errorType RqErrorType, message string) RqError {
	job.nFails += 1
	return RqError{
		job:       job,
		errorType: errorType,
		errorMsg:  message,
	}
}

// Create a new pipeline; if nWorkers <= 0, run download and process sync
func NewPipeline(nWorkers int) *RqPipeline {
	nWorkers = int(math.Max(1, float64(nWorkers)))
	pool := RqPool{
		nWorkers:     nWorkers,
		wg:           sync.WaitGroup{},
		downloadChn:  make(chan RqJob),
		summarizeChn: make(chan RqJob),
		cleanupChn:   make(chan RqJob),
		saveChn:      make(chan RqJob),
		errorChn:     make(chan RqError),
		doneChn:      make(chan int),
		client:       newClient(defaultTimeout),
		stopOnce:     sync.Once{},
	}

	return &RqPipeline{
		pool:       &pool,
		sourceURLs: nil,
		outFile:    nil,
		imageCount: 0,
	}
}

func (pipe *RqPipeline) WithSource(imageURLs io.Reader) *RqPipeline {
	pipe.sourceURLs = imageURLs
	return pipe
}

func (pipe *RqPipeline) WithClient(client *http.Client) *RqPipeline {
	pipe.pool.client = client
	return pipe
}

func (pipe *RqPipeline) WithOutput(out io.Writer) *RqPipeline {
	pipe.outFile = out
	return pipe
}

func (pipe *RqPipeline) Init() (*RqPipeline, error) {
	if pipe.sourceURLs == nil {
		return pipe, errors.New("Pipeline has no source set. Use method WithSource to set it.")
	}
	if pipe.outFile == nil {
		return pipe, errors.New("Pipeline has no output file set. Use method WithSource to set it.")
	}

	return pipe, nil
}

// Read lines of URLs into images and send into the downloadChn; NOT thread safe
func (pipe *RqPipeline) readURLs() {
	scanner := bufio.NewScanner(pipe.sourceURLs)
	for scanner.Scan() {
		imgURL := strings.TrimSpace(scanner.Text())
		atomic.AddUint64(&pipe.imageCount, 1)
		log.Printf("Starting %v", imgURL)
		pipe.pool.downloadChn <- RqJob{
			image:    NewRqImage(imgURL),
			retryChn: nil,
			nextChn:  nil,
		}
	}
	pipe.mux.Lock()
	defer pipe.mux.Unlock()
	pipe.readURLsDone = true
}

// Write results from the saveChn to the output file; NOT thread safe
func (pipe *RqPipeline) writeResults() {
	for job := range pipe.pool.saveChn {
		line := []string{job.image.URL}
		line = append(line, job.image.GetHexSummary()...)
		_, err := pipe.outFile.Write([]byte(strings.Join(line, ",")))
		if err != nil {
			pipe.pool.errorChn <- NewRqError(job, RqErrorNoRetry, err.Error())
			continue
		}
		atomic.AddUint64(&pipe.imageCount, ^uint64(0))

		log.Printf("Finished %v", job.image.URL)

		if pipe.isDone() {
			pipe.pool.stopWorkers()
			return
		}
	}
}

// Handles job errors by requeuing them or removing them from the pipeline
func (pipe *RqPipeline) handleError(jobError RqError) {
	if jobError.errorType == RqErrorNoRetry || jobError.job.nFails >= RqJobMaxFails {
		log.Printf("Job Failed: %v\n", jobError.errorMsg)
		// cleanup image - ignore failures to prevent infinite loop
		cleanupImage(jobError.job, nil)
		atomic.AddUint64(&pipe.imageCount, ^uint64(0))
		if pipe.isDone() {
			pipe.pool.stopWorkers()
		}
		return
	}

	log.Printf("Job Error(%v): %v: %v\n", jobError.errorType, jobError.job.image.URL, jobError.errorMsg)
	jobError.job.retryChn <- jobError.job
}

func (pipe *RqPipeline) isDone() bool {
	pipe.mux.Lock()
	defer pipe.mux.Unlock()
	return pipe.readURLsDone && pipe.imageCount == 0
}

func (pool *RqPool) stopWorkers() {
	pool.stopOnce.Do(func() {
		for i := 0; i < pool.nWorkers; i += 1 {
			pool.doneChn <- 1
		}
	})
}

// close all channels used by the pool
func (pool *RqPool) closeChns() {
	close(pool.downloadChn)
	close(pool.summarizeChn)
	close(pool.cleanupChn)
	close(pool.saveChn)
	close(pool.errorChn)
	close(pool.doneChn)
}

func (pipe *RqPipeline) Run() {
	// goroutine to read source file into channel
	go pipe.readURLs()

	// goroutine to write results
	go pipe.writeResults()

	// kickoff workers
	for i := 0; i < pipe.pool.nWorkers-1; i += 1 {
		pipe.pool.wg.Add(1)
		go pipe.work()
	}

	// send main goroutine to do work
	pipe.pool.wg.Add(1)
	pipe.work()

	pipe.pool.wg.Wait()
	pipe.pool.closeChns()
}

// worker function
func (pipe *RqPipeline) work() {
	defer pipe.pool.wg.Done()
	pool := pipe.pool

	for {
		select {
		case job := <-pool.downloadChn:
			job.retryChn = pool.downloadChn
			job.nextChn = pool.summarizeChn
			downloadImage(job, pool.client, pool.errorChn)

		case job := <-pool.summarizeChn:
			job.retryChn = pool.summarizeChn
			job.nextChn = pool.cleanupChn
			summarizeImage(job, pool.errorChn)

		case job := <-pool.cleanupChn:
			job.retryChn = pool.cleanupChn
			job.nextChn = pool.saveChn
			cleanupImage(job, pool.errorChn)

		case jobError := <-pool.errorChn:
			pipe.handleError(jobError)

		case <-pool.doneChn:
			return

		default:
			// log.Println("whoops")
		}
	}
}

// Download an image from its url
func downloadImage(job RqJob, client *http.Client, errorChn chan<- RqError) {
	tmpFile, err := ioutil.TempFile("", "*.tmpimg")
	if err != nil {
		errorChn <- NewRqError(job, RqErrorDownload, err.Error())
		return
	}
	defer tmpFile.Close()

	img := job.image
	err = downloadToFile(img.URL, tmpFile, client)
	if err != nil {
		errorChn <- NewRqError(job, RqErrorDownload, err.Error())
		return
	}
	job.image.filePath = tmpFile.Name()

	log.Printf("Downloaded %v", job.image.URL)
	job.nextChn <- job
}

// Open an image and calculate the most frequent colors
func summarizeImage(job RqJob, errorChn chan<- RqError) {
	img := job.image
	imgFile, err := os.Open(img.filePath)
	if err != nil {
		errorChn <- NewRqError(job, RqErrorSummarize, err.Error())
		return
	}
	defer imgFile.Close()

	imgImage, _, err := image.Decode(imgFile)
	if err != nil {
		errorChn <- NewRqError(job, RqErrorSummarize, err.Error())
		return
	}

	summary, err := getPrevalentColors(&imgImage)
	if err != nil {
		errorChn <- NewRqError(job, RqErrorSummarize, err.Error())
		return
	}

	job.image.summary = summary
	log.Printf("Summarized %v", job.image.URL)
	job.nextChn <- job
}

// Delete an image
func cleanupImage(job RqJob, errorChn chan<- RqError) {
	if job.image.filePath == "" {
		// image wasn't downloaded
		job.nextChn <- job
		return
	}

	err := os.Remove(job.image.filePath)
	if err != nil && errorChn != nil {
		errorChn <- NewRqError(job, RqErrorCleanup, err.Error())
		return
	}

	job.image.filePath = ""
	log.Printf("Cleaned %v", job.image.URL)
	job.nextChn <- job
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

	imagesFile, err := os.Open(imagesPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer imagesFile.Close()

	pipeline, err := NewPipeline(20).
		WithSource(imagesFile).
		WithOutput(csvoutFile).
		Init()
	if err != nil {
		log.Fatalln(err)
	}
	pipeline.Run()
}
