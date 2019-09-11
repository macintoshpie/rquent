# rquent
## tldr;
command line tool for getting the most frequent colors from images

## Setup
If you have go installed, you can clone the repo, build it, and run it.
```
git clone git@github.com:macintoshpie/rquent.git &&
  cd rquent &&
  go build . &&
  ./rquent -urls <path to file of image urls>
```

## Usage
Run the command `./rquent` to see the help.
The source file of image urls is assumed to have one url on each line

## Comments
### Calculating most frequent color
The function `getPrevalentColors` in image.go returns the 3 most prevalent colors in an image. It does this by iterating over the pixels and updating counts in a map indexed by color.
I considered parallelizing the processing of a single image by creating separate maps and then merging them, but I don't think that'd be very useful on a single core machine.  
I noticed it's costly to convert to NRGBA colors, and I tried converting the whole image at once rather than pixel by pixel, but it turned out to be slower.
#### Possible Improvements
- Don't use a map - use a trie as nested arrays. This should be much faster than accessing and updating a map (see comments in Testing section below)
- if 100% correctness isn't important (which it probably isn't) I'd resize the images before processing them. This would save an insane amount of time
- I'd do more research into k means clustering - seems relevant but not sure about its performance
- I would possibly have multiple workers opening images and sending blocks to a single routine that calculates frequencies from those blocks, as this could save some io time opening images. This would use a lot more memory however
- if we wanted to generalize to the k most prevalent, I would use a min heap rather than manually updating my slice storing the top 3
- don't cast to NRGBA, just do your own conversions to determine RGB values.
### Pipeline
The pipeline is broken down into reading the source file, downloading images, processing images, cleaning up images, saving results, and handling any failed steps. Here's a diagram (note the arrows to the errorHandler are bidirectional - ie requeued)
```
  url file
      |
      V
+----------+
|readSource+----------------+
+-----+----+                |
      |                     |
+-----v---------+           |
|downloadImages +--------+  |
+-----+---------+        |  |
      |                  v  v
+-----v---------+       ++--+--------+
|summarizeImages+------>+errorHandler|
+-----+---------+       ++--+--+-----+
      |                  ^  ^  |
+-----v--------+         |  |  |
|cleanupImages +---------+  |  |
+-----+--------+            |  |
      |                     |  |
+-----v-----+               |  |
|saveResults+---------------+  |
+-----+-----+                  |
      |                        |
      |     +------+           |
      +---->+ DONE +<----------+
            +------+

```
`RqPipeline.Run()` spins up the desired number of workers for connecting these channels.  
Each image is represented as a "job" throughout the pipeline, keeping track of it's url, file path, and number of fails.  
If there's an error at some step, we create an error into the error channel, which is then handled. If the job has failed too many times, it exits the pipeline, otherwise, it's requeued into the channel that originally was trying to process it.  
Having more workers in the download function is important because the async nature of the process, while processing images is cpu bound.  

The number of workers for each section is configurable from the command line, and if I had more time I would have run tests to determine which was the best.  
Also, my pipeline doesn't really take image size into consideration when loading them into memory, which could become problematic if run with more summarizing workers on a machine with more cores. To fix this I would keep some global state which tracked currently opened images and their sizes, then only open images which could fit.  
My pipeline also doesn't track the size of images downloaded currently - as a result it's imaginable you'd run out of disk space with large enough images and many downloading workers. It'd be easy to just do a HEAD request, update the size of the image from `Content-length`, then do some handling with that info.  
#### Possible improvements
- handling errors could be done better, specifically how they are reported. It'd be nice to have the failed image's url saved as well as why it failed.
- depending on the source of URLs, caching could be extremely valuable.

### Testing/Benchmarking
I wrote a decent number of tests for the image processing specifically as well as the pipeline.  
In addition, I created a simple image server for performance testing, ([repo here](https://github.com/macintoshpie/fileserver)). I used a delay of 100ms for responses.  
For testing, I ran `rquent` in a resource limited container (1 core, 512mb memory) with default worker configuration.  
The results are not very good, it would be impossible to use this system in production with 1 billion urls as it'd take forever. I would make the improvements I talked about to make this much faster.
#### Provided images file (1000 urls)
I ran this test on the data provided by the coding challenge prompt. The tool was able to process about 2 images per second. Here are the timing results:
```
real    8m7.820s
user    0m0.181s
sys     0m0.241s
```

I ran it once more with profiling to get the cpu usage:
```
Showing nodes accounting for 4520ms, 71.52% of 6320ms total
Dropped 88 nodes (cum <= 31.60ms)
Showing top 10 nodes out of 93
      flat  flat%   sum%        cum   cum%
     930ms 14.72% 14.72%     1260ms 19.94%  runtime.mapassign_fast32
     640ms 10.13% 24.84%      640ms 10.13%  runtime.aeshash32
     550ms  8.70% 33.54%      550ms  8.70%  syscall.Syscall
     510ms  8.07% 41.61%      510ms  8.07%  runtime.(*mspan).refillAllocCache
     440ms  6.96% 48.58%      900ms 14.24%  runtime.mapaccess1_fast32
     400ms  6.33% 54.91%      450ms  7.12%  image.(*YCbCr).YCbCrAt
     360ms  5.70% 60.60%     1420ms 22.47%  runtime.mallocgc
     340ms  5.38% 65.98%     5140ms 81.33%  main.getPrevalentColors
     200ms  3.16% 69.15%      200ms  3.16%  runtime.(*mspan).init (inline)
     150ms  2.37% 71.52%      150ms  2.37%  image/color.YCbCr.RGBA
```
It looks like updating my map of color counts is really slow. I would consider using an array instead, and make a trie.  
That'd give constant time access and would take up a bit more than 1 billion bytes (255^3 * 64), so I'm pretty confident we could fit it in 512 mb. In fact, I'm really underutilizing memory, looking at the container stats I'm only using about 20% of available memory.
#### 100 images
Tested using my mock image server, with about 20 different images averaging about 100KB in size.  
rquent averaged about 3 images per second.
```
real    0m27.665s
user    0m0.045s
sys     0m0.038s

real    0m27.772s
user    0m0.049s
sys     0m0.036s

real    0m28.123s
user    0m0.047s
sys     0m0.037s
```

#### Further testing
- use my mock server to test a wide range of image sizes and response speeds.
- test different numbers of workers for each section of pipeline