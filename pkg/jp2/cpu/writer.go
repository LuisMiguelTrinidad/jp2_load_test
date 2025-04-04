package cpu

import (
	"errors"
	"fmt"
	"image"
	"os"
	"time"
	"unsafe"
)

// #cgo CFLAGS: -I/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/include
// #cgo LDFLAGS: -L/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/lib -lopenjp2
// #include <openjpeg-2.5/openjpeg.h>
// #include <stdlib.h>
import "C"

// Writer implements jp2.Writer for CPU-based encoding
type Writer struct {
	outputPath string
}

// NewWriter creates a new CPU-based JP2 writer
func NewWriter(outputPath string) *Writer {
	return &Writer{outputPath: outputPath}
}

// Update the Write method to match the jp2.Writer interface
func (w *Writer) Write(img *image.RGBA, outputPath string, threads int) (time.Duration, error) {
	startSave := time.Now()

	// Create directory if it doesn't exist
	if err := os.MkdirAll("./go_jp2_direct", 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %v", err)
	}

	// Use the outputPath argument for the output filename
	cfilename := C.CString(outputPath)
	defer C.free(unsafe.Pointer(cfilename))

	// Get image dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pix := img.Pix

	// Configure component parameters (RGBA)
	var cmptparm [4]C.opj_image_cmptparm_t
	for i := 0; i < 4; i++ {
		cmptparm[i].dx = 1
		cmptparm[i].dy = 1
		cmptparm[i].w = C.uint(width)
		cmptparm[i].h = C.uint(height)
		cmptparm[i].x0 = 0
		cmptparm[i].y0 = 0
		cmptparm[i].prec = 8
		cmptparm[i].sgnd = 0
	}

	// Create OpenJPEG image
	cimage := C.opj_image_create(4, &cmptparm[0], C.OPJ_CLRSPC_SRGB)
	if cimage == nil {
		return 0, errors.New("failed to create OpenJPEG image")
	}
	defer C.opj_image_destroy(cimage)

	// Configure origin coordinates
	cimage.x0 = 0
	cimage.y0 = 0
	cimage.x1 = C.uint(width)
	cimage.y1 = C.uint(height)

	// Copy RGBA data to OpenJPEG components
	pixelCount := width * height
	for i := 0; i < 4; i++ {
		comp := (*C.opj_image_comp_t)(unsafe.Pointer(
			uintptr(unsafe.Pointer(cimage.comps)) + uintptr(i)*unsafe.Sizeof(C.opj_image_comp_t{}),
		))

		if comp.data == nil {
			return 0, fmt.Errorf("component %d data is nil", i)
		}

		// Safer approach with explicit scope
		{
			data := unsafe.Slice((*C.int)(comp.data), pixelCount)
			for j := 0; j < pixelCount; j++ {
				pixIndex := j*4 + i
				if pixIndex >= len(pix) {
					return 0, fmt.Errorf("index out of range: pixel %d, component %d", j, i)
				}
				data[j] = C.int(pix[pixIndex])
			}
			// Force data to be nil to break reference
			data = nil
		}
	}

	// Configure encoder parameters
	var parameters C.opj_cparameters_t
	C.opj_set_default_encoder_parameters(&parameters)

	// Configuration for JP2 encoding
	parameters.tcp_numlayers = 1
	parameters.tcp_rates[0] = 0
	parameters.irreversible = 0
	parameters.numresolution = 6
	parameters.cp_disto_alloc = 1

	// Create JP2 codec
	codec := C.opj_create_compress(C.OPJ_CODEC_JP2)
	if codec == nil {
		return 0, errors.New("failed to create JP2 codec")
	}
	defer C.opj_destroy_codec(codec)

	// Set up the encoder with the image and parameters
	if C.opj_setup_encoder(codec, &parameters, cimage) == C.OPJ_FALSE {
		return 0, errors.New("failed to setup encoder")
	}

	// Configure threads if more than 1 is specified
	if threads > 1 {
		if C.opj_codec_set_threads(codec, C.int(threads)) == C.OPJ_FALSE {
			fmt.Printf("Warning: Could not configure %d threads for encoding\n", threads)
		}
	}

	// Create and configure a stream for writing
	stream := C.opj_stream_create_default_file_stream(cfilename, 0)
	if stream == nil {
		return 0, fmt.Errorf("failed to create file stream for %s", w.outputPath)
	}
	defer C.opj_stream_destroy(stream)

	// Start the encoding process
	if C.opj_start_compress(codec, cimage, stream) == C.OPJ_FALSE {
		return 0, errors.New("failed to start compression")
	}

	// Encode the image
	if C.opj_encode(codec, stream) == C.OPJ_FALSE {
		return 0, errors.New("failed to encode image")
	}

	// End the encoding process
	if C.opj_end_compress(codec, stream) == C.OPJ_FALSE {
		return 0, errors.New("failed to end compression")
	}

	saveTime := time.Since(startSave)
	return saveTime, nil
}
