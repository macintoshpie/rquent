package main

import (
	"flag"
	_ "image/jpeg"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

func main() {
	var imagesPath *string = flag.String("urls", "", "source file for images (required)")
	var csvoutPath *string = flag.String("out", "results.csv", "destination for results")
	var nDownload *int = flag.Int("download", 10, "number of workers downloading images")
	var nSummarize *int = flag.Int("summarize", 2, "number of workers summarizing images")
	var nCleanup *int = flag.Int("cleanup", 2, "number of workers cleaning up images")
	var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
	var memprofile = flag.String("memprofile", "", "write memory profile to `file`")

	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	// Setup input and output files
	csvoutFile, err := os.Create(*csvoutPath)
	if err != nil {
		log.Printf("Failed to open output file (%v): %v", csvoutPath, err)
		flag.Usage()
		return
	}
	defer csvoutFile.Close()

	imagesFile, err := os.Open(*imagesPath)
	if err != nil {
		log.Printf("Failed to open source file (%v): %v", imagesPath, err)
		flag.Usage()
		return
	}
	defer imagesFile.Close()

	// Create and configure the pipeline
	pipeCfg := PipeConfig{*nDownload, *nSummarize, *nCleanup}
	pipeline, err := NewPipeline(pipeCfg).
		WithSource(imagesFile).
		WithOutput(csvoutFile).
		Init()
	if err != nil {
		log.Fatalln(err)
	}

	// Run it
	pipeline.Run()

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
}
