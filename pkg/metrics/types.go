package metrics

import "time"

// Metrics contains all the metrics for the NDVI processing
type Metrics struct {
	Resolution    string
	ProcessorType string // "CPU" or "GPU"
	NumThreads    int    // Number of threads used (for CPU)
	TotalTime     time.Duration
	ReadingTime   time.Duration
	NDVITime      time.Duration
	ColorTime     time.Duration
	SaveTime      time.Duration
	FileTimeNIR   time.Duration
	DecodeTimeNIR time.Duration
	FileTimeRED   time.Duration
	DecodeTimeRED time.Duration
	Pixels        int
	NoDataPixels  int
	ImageSize     int64
	NDVIMin       float64
	NDVIMax       float64
	NDVIAverage   float64
	NumTilesNIR   int
	NumTilesRED   int
	CPUMetrics    CPUMetrics
}

// ReadMetrics contains metrics associated with reading a JP2 file
type ReadMetrics struct {
	FileTime    time.Duration
	DecodeTime  time.Duration
	NumTiles    int
	TotalTime   time.Duration
	ParseTime   time.Duration
	GetInfoTime time.Duration
}

// NDVIMetrics contains metrics for NDVI calculation
type NDVIMetrics struct {
	Time         time.Duration
	TotalPixels  int
	NoDataPixels int
	Min          float64
	Max          float64
	Average      float64
}

// ColorMetrics contains metrics for colorization
type ColorMetrics struct {
	Time      time.Duration
	ImageSize int64
}

// CPUMetrics contains CPU-specific time metrics
type CPUMetrics struct {
	FileTime   time.Duration
	DecodeTime time.Duration
	TotalTime  time.Duration
}
