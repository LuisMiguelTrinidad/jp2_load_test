#!/bin/bash
# Build the project with optimizations

echo "Building the project with optimizations..."
CGO_CFLAGS="-O3" go build -o jp2_processing ./cmd/benchmark

echo "Build completed with optimizations. Executable created: ./jp2_processing"