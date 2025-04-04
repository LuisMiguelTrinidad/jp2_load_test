package ndvi

import (
	"math"
	"sync"

	"github.com/luismi/jp2_processing/pkg/jp2"
	"github.com/luismi/jp2_processing/pkg/metrics"
)

// Calculate computes NDVI values from NIR and RED bands
// Returns NDVI metrics, float64 array with NDVI values, and any error
func Calculate(nirBand, redBand *jp2.BandResult, numThreads int) (*metrics.NDVIMetrics, []float64, error) {
	ndviMetrics := &metrics.NDVIMetrics{
		Min: math.MaxFloat64,
		Max: -math.MaxFloat64,
	}

	// Verify dimensions are equal
	if nirBand.Image.Width != redBand.Image.Width || nirBand.Image.Height != redBand.Image.Height {
		return nil, nil, &ImageDimensionError{
			NIRWidth:  nirBand.Image.Width,
			NIRHeight: nirBand.Image.Height,
			REDWidth:  redBand.Image.Width,
			REDHeight: redBand.Image.Height,
		}
	}

	// Get dimensions
	width := nirBand.Image.Width
	height := redBand.Image.Height
	pixelCount := width * height
	ndviMetrics.TotalPixels = pixelCount

	// Calculate NDVI in parallel
	ndviData := make([]float64, pixelCount)

	// Setup parallel processing
	numWorkers := numThreads
	chunkSize := (pixelCount + numWorkers - 1) / numWorkers
	minVals := make([]float64, numWorkers)
	maxVals := make([]float64, numWorkers)
	sums := make([]float64, numWorkers)
	pixelsWithoutData := make([]int, numWorkers)

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	nirData := nirBand.Image.Data[0]
	redData := redBand.Image.Data[0]

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := min(start+chunkSize, pixelCount)

		go func(worker, start, end int) {
			defer wg.Done()
			localMin := math.MaxFloat64
			localMax := -math.MaxFloat64
			localSum := 0.0
			localNoData := 0

			for i := start; i < end; i++ {
				nirVal := float64(nirData[i])
				redVal := float64(redData[i])

				sum := nirVal + redVal
				ndviValue := 0.0
				if sum > 0 {
					ndviValue = (nirVal - redVal) / sum
				} else {
					// Count as no-data pixel
					localNoData++
				}
				ndviData[i] = ndviValue

				if ndviValue < localMin {
					localMin = ndviValue
				}
				if ndviValue > localMax {
					localMax = ndviValue
				}
				localSum += ndviValue
			}

			minVals[worker] = localMin
			maxVals[worker] = localMax
			sums[worker] = localSum
			pixelsWithoutData[worker] = localNoData
		}(w, start, end)
	}

	wg.Wait()

	// Calculate NDVI metrics
	ndviMetrics.Min = minVals[0]
	ndviMetrics.Max = maxVals[0]
	totalSum := sums[0]

	for i := 1; i < numWorkers; i++ {
		if minVals[i] < ndviMetrics.Min {
			ndviMetrics.Min = minVals[i]
		}
		if maxVals[i] > ndviMetrics.Max {
			ndviMetrics.Max = maxVals[i]
		}
		totalSum += sums[i]
	}

	ndviMetrics.Average = totalSum / float64(pixelCount)

	// Count pixels without data
	totalWithoutData := 0
	for _, count := range pixelsWithoutData {
		totalWithoutData += count
	}
	ndviMetrics.NoDataPixels = totalWithoutData

	return ndviMetrics, ndviData, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ImageDimensionError is returned when NIR and RED images have different dimensions
type ImageDimensionError struct {
	NIRWidth, NIRHeight, REDWidth, REDHeight int
}

func (e *ImageDimensionError) Error() string {
	return "NIR and RED images have different dimensions"
}
