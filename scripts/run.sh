#!/bin/bash
# Run the project with default configurations

# Default configurations
NIR_FILE="test_images/B08_20m.jp2"
RED_FILE="test_images/B04_20m.jp2"
THREADS="2,4,8,12,16"
ITERATIONS=1

# Run the benchmark
./jp2_processing -nir $NIR_FILE -red $RED_FILE -threads $THREADS -iter $ITERATIONS