package jp2

import (
	"image"
	"time"
)

// Writer is the interface for JPEG2000 image writers
type Writer interface {
	// Write encodes and saves an RGBA image as JPEG2000
	// Returns the time taken to write the image and any error that occurred
	Write(img *image.RGBA, resolution string, threads int) (time.Duration, error)
}
