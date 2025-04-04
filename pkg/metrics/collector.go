package metrics

import (
	"time"
)

// Collector handles collecting and managing metrics for NDVI processing
type Collector struct {
	metrics       *Metrics
	processorType string
	numThreads    int
}

// NewCollector creates a new metrics collector
func NewCollector(processorType string, numThreads int) *Collector {
	return &Collector{
		metrics: &Metrics{
			ProcessorType: processorType,
			NumThreads:    numThreads,
		},
		processorType: processorType,
		numThreads:    numThreads,
	}
}

// StartTiming starts measuring total time
func (c *Collector) StartTiming() time.Time {
	return time.Now()
}

// StopTiming stops measuring total time
func (c *Collector) StopTiming(start time.Time) {
	c.metrics.TotalTime = time.Since(start)
}

// SetNumTiles sets the number of tiles for NIR and RED bands
func (c *Collector) SetNumTiles(nirTiles, redTiles int) {
	c.metrics.NumTilesNIR = nirTiles
	c.metrics.NumTilesRED = redTiles
}

// SetBandReadMetrics sets metrics related to reading band files
func (c *Collector) SetBandReadMetrics(nirMetrics, redMetrics *ReadMetrics) {
	c.metrics.FileTimeNIR = nirMetrics.FileTime
	c.metrics.DecodeTimeNIR = nirMetrics.DecodeTime
	c.metrics.FileTimeRED = redMetrics.FileTime
	c.metrics.DecodeTimeRED = redMetrics.DecodeTime

	// Total reading time
	c.metrics.ReadingTime = nirMetrics.TotalTime + redMetrics.TotalTime
}

// SetNDVIMetrics sets metrics related to NDVI calculation
func (c *Collector) SetNDVIMetrics(ndviMetrics *NDVIMetrics, time time.Duration) {
	c.metrics.NDVITime = time
	c.metrics.Pixels = ndviMetrics.TotalPixels
	c.metrics.NoDataPixels = ndviMetrics.NoDataPixels
	c.metrics.NDVIMin = ndviMetrics.Min
	c.metrics.NDVIMax = ndviMetrics.Max
	c.metrics.NDVIAverage = ndviMetrics.Average
}

// SetColorMetrics sets metrics related to colorization
func (c *Collector) SetColorMetrics(colorMetrics *ColorMetrics, time time.Duration) {
	c.metrics.ColorTime = time
	c.metrics.ImageSize = colorMetrics.ImageSize
}

// SetSaveTime sets the time spent saving the image
func (c *Collector) SetSaveTime(time time.Duration) {
	c.metrics.SaveTime = time
}

// GetMetrics returns the collected metrics
func (c *Collector) GetMetrics() *Metrics {
	return c.metrics
}
