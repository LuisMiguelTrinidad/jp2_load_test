package ndvi

import (
	"image"
	"sync"

	"github.com/luismi/jp2_processing/config"
	"github.com/luismi/jp2_processing/pkg/metrics"
)

// Colorize converts NDVI values to a color image
func Colorize(ndviData []float64, width, height int, numThreads int) (*metrics.ColorMetrics, *image.RGBA) {
	colorMetrics := &metrics.ColorMetrics{}

	// Create output RGBA image
	ndviColorImg := image.NewRGBA(image.Rect(0, 0, width, height))
	colorPix := ndviColorImg.Pix

	pixelCount := width * height

	// Process color in parallel
	numWorkers := numThreads
	chunkSize := (pixelCount + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := min(start+chunkSize, pixelCount)

		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				rgba := config.GetNDVIColor(ndviData[i])
				idx := i * 4
				colorPix[idx] = rgba.R
				colorPix[idx+1] = rgba.G
				colorPix[idx+2] = rgba.B
				colorPix[idx+3] = 255 // Full opacity
			}
		}(start, end)
	}

	wg.Wait()

	// Set image size in bytes (4 bytes per pixel RGBA)
	colorMetrics.ImageSize = int64(width * height * 4)

	return colorMetrics, ndviColorImg
}
