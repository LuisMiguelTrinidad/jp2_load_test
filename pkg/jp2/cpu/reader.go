package cpu

import (
	"fmt"
	"math"
	"time"
	"unsafe"

	"github.com/luismi/jp2_processing/internal/cgo/openjpeg"
	"github.com/luismi/jp2_processing/pkg/jp2"
	"github.com/luismi/jp2_processing/pkg/metrics"
)

// #cgo CFLAGS: -I/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/include
// #cgo LDFLAGS: -L/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/lib -lopenjp2
// #include <openjpeg-2.5/openjpeg.h>
// #include <stdlib.h>
import "C"

// Reader implements jp2.Reader for CPU-based decoding
type Reader struct{}

// NewReader creates a new CPU-based JP2 reader
func NewReader() *Reader {
	return &Reader{}
}

// Read implements the jp2.Reader interface
func (r *Reader) Read(filePath string, threads int) (*jp2.BandResult, error) {
	result := &jp2.BandResult{
		Metrics: metrics.ReadMetrics{},
	}

	startTotal := time.Now()

	// Measure file opening time
	startFile := time.Now()

	// Convert Go string to C string
	cFilePath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cFilePath))

	// Configure the stream
	stream := C.opj_stream_create_default_file_stream(cFilePath, 1)
	if stream == nil {
		return nil, fmt.Errorf("could not open file: %s", filePath)
	}
	defer C.opj_stream_destroy(stream)

	result.Metrics.FileTime = time.Since(startFile)

	// Measure decoding time
	startDecode := time.Now()

	// Create the codec
	codec := C.opj_create_decompress(C.OPJ_CODEC_JP2)
	if codec == nil {
		return nil, fmt.Errorf("could not create JP2 codec")
	}
	defer C.opj_destroy_codec(codec)

	// Configure callbacks
	errorCb, warningCb, infoCb := openjpeg.GetCallbacks()
	C.opj_set_error_handler(codec, C.opj_msg_callback(errorCb), nil)
	C.opj_set_warning_handler(codec, C.opj_msg_callback(warningCb), nil)
	C.opj_set_info_handler(codec, C.opj_msg_callback(infoCb), nil)

	// Configure parameters
	parameters := C.opj_dparameters_t{}
	C.opj_set_default_decoder_parameters(&parameters)

	if C.opj_setup_decoder(codec, &parameters) == C.OPJ_FALSE {
		return nil, fmt.Errorf("error setting up decoder")
	}

	// Configure number of threads if more than 1 is specified
	if threads > 1 {
		if C.opj_codec_set_threads(codec, C.int(threads)) == C.OPJ_FALSE {
			fmt.Printf("Warning: Could not configure %d threads, continuing with default configuration\n", threads)
		} else {
			fmt.Printf("Decoding with %d threads\n", threads)
		}
	}

	// Read the header
	var image *C.opj_image_t
	if C.opj_read_header(stream, codec, &image) == C.OPJ_FALSE {
		C.opj_destroy_codec(codec)
		C.opj_stream_destroy(stream)
		return nil, fmt.Errorf("error reading image header")
	}

	// Get information about tiles after reading the header
	var numTiles int = 1 // Default value if info can't be obtained
	cstrInfo := C.opj_get_cstr_info(codec)
	if cstrInfo != nil {
		numTiles = int(cstrInfo.tw * cstrInfo.th)
		C.opj_destroy_cstr_info(&cstrInfo)
	}

	// Decode the image
	if C.opj_decode(codec, stream, image) == C.OPJ_FALSE {
		C.opj_image_destroy(image)
		return nil, fmt.Errorf("error decoding image")
	}

	// Convert OpenJPEG image to our format
	jp2Image := &jp2.JP2Image{
		Width:      int(image.x1 - image.x0),
		Height:     int(image.y1 - image.y0),
		Components: int(image.numcomps),
		Data:       make([][]float32, int(image.numcomps)),
	}

	// Extract data from each component
	for i := 0; i < jp2Image.Components; i++ {
		comp := *(*C.opj_image_comp_t)(unsafe.Pointer(
			uintptr(unsafe.Pointer(image.comps)) + uintptr(i)*unsafe.Sizeof(C.opj_image_comp_t{}),
		))

		// Allocate memory for this component's data
		jp2Image.Data[i] = make([]float32, jp2Image.Width*jp2Image.Height)

		// Get component data pointer once outside the loop
		compData := (*[1 << 30]C.int)(unsafe.Pointer(comp.data))[: jp2Image.Width*jp2Image.Height : jp2Image.Width*jp2Image.Height]

		// Copy normalized data
		factor := math.Pow(2, float64(comp.prec)-1)
		for j := range jp2Image.Width * jp2Image.Height {
			// Normalize to [0,1]
			jp2Image.Data[i][j] = float32(float64(compData[j]) / factor)
		}
	}

	// Free image memory
	C.opj_image_destroy(image)

	result.Metrics.DecodeTime = time.Since(startDecode)
	result.Metrics.NumTiles = numTiles
	result.Metrics.TotalTime = time.Since(startTotal)
	result.Image = jp2Image

	return result, nil
}
