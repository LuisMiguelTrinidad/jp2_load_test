package jp2

import (
	"github.com/luismi/jp2_processing/pkg/metrics"
)

// JP2Image represents a decoded JPEG2000 image
type JP2Image struct {
	Width, Height int
	Components    int
	Data          [][]float32 // One slice per component
}

// BandResult contains a JP2 image and its reading metrics
type BandResult struct {
	Image   *JP2Image
	Metrics metrics.ReadMetrics
}

// Reader is the interface for JPEG2000 image readers
type Reader interface {
	// Read reads a JPEG2000 image file and returns the image data and metrics
	Read(filePath string, threads int) (*BandResult, error)
}

// Free releases memory used by JP2Image
func (img *JP2Image) Free() {
	if img == nil {
		return
	}
	for i := range img.Data {
		// Setting to nil is enough for Go's garbage collector
		img.Data[i] = nil
	}
}

// Free releases memory used by BandResult
func (rb *BandResult) Free() {
	if rb == nil {
		return
	}

	// Free the JP2 image
	if rb.Image != nil {
		rb.Image.Free()
		rb.Image = nil
	}
}
