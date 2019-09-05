#!/bin/bash

nMem="512mb"
nCPU="1"

if [[ $1 =~ "run" ]]; then
  docker run -it --rm --cpus=$nCPU --memory=$nMem rquent app tmp.csv result.csv
elif [[ $1 =~ "build" ]]; then
  docker build -t rquent .
elif [[ $1 =~ "bench" ]]; then
  docker run -it --rm --cpus=$nCPU --memory=$nMem rquent go test -bench=.
fi