package gpu

import (
	"errors"
	"fmt"
	"math"
	"time"
	"unsafe"

	"github.com/luismi/jp2_processing/pkg/jp2"
	"github.com/luismi/jp2_processing/pkg/metrics"
)

// #cgo LDFLAGS: -L/usr/local/cuda/lib64 -lcudart -lnvjpeg2k
// #include "nvjpeg2k.h"
// #include <cuda_runtime.h>
// #include <stdlib.h>
import "C"

// Reader implements jp2.Reader for GPU-based decoding
type Reader struct{}

// NewReader creates a new GPU-based JP2 reader
func NewReader() *Reader {
	return &Reader{}
}

// Read implements the jp2.Reader interface
// The threads parameter is kept for compatibility with the CPU reader but has no effect
func (r *Reader) Read(filePath string, threads int) (*jp2.BandResult, error) {
	result := &jp2.BandResult{
		Metrics: metrics.ReadMetrics{},
	}

	startTotal := time.Now()

	// Initialize CUDA and nvjpeg2k
	var handle C.nvjpeg2kHandle_t
	if status := C.nvjpeg2kCreateSimple(&handle); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to create nvjpeg2k handle: %v", status)
	}
	defer C.nvjpeg2kDestroy(handle)

	var stream C.nvjpeg2kStream_t
	if status := C.nvjpeg2kStreamCreate(&stream); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to create nvjpeg2k stream: %v", status)
	}
	defer C.nvjpeg2kStreamDestroy(stream)

	// Parse the file
	startFile := time.Now()
	cFilePath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cFilePath))

	if status := C.nvjpeg2kStreamParseFile(handle, cFilePath, stream); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to parse file: %v", status)
	}

	// Get image information
	var imageInfo C.nvjpeg2kImageInfo_t
	if status := C.nvjpeg2kStreamGetImageInfo(stream, &imageInfo); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to get image info: %v", status)
	}

	result.Metrics.FileTime = time.Since(startFile)

	// Decoding
	startDecode := time.Now()

	numComponents := int(imageInfo.num_components)
	var pixelType C.nvjpeg2kImageType_t

	// Determine pixel type based on the first component
	var compInfo C.nvjpeg2kImageComponentInfo_t
	if status := C.nvjpeg2kStreamGetImageComponentInfo(stream, &compInfo, 0); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to get component info: %v", status)
	}

	switch {
	case compInfo.precision <= 8 && compInfo.sgn == 0:
		pixelType = C.NVJPEG2K_UINT8
	case compInfo.precision <= 16 && compInfo.sgn == 0:
		pixelType = C.NVJPEG2K_UINT16
	case compInfo.precision <= 16 && compInfo.sgn != 0:
		pixelType = C.NVJPEG2K_INT16
	default:
		return nil, errors.New("unsupported component precision/sign")
	}

	// Decoding parameters configuration
	var decodeParams C.nvjpeg2kDecodeParams_t
	if status := C.nvjpeg2kDecodeParamsCreate(&decodeParams); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to create decode params: %v", status)
	}
	defer C.nvjpeg2kDecodeParamsDestroy(decodeParams)

	C.nvjpeg2kDecodeParamsSetOutputFormat(decodeParams, C.NVJPEG2K_FORMAT_PLANAR)

	var decodeState C.nvjpeg2kDecodeState_t
	if status := C.nvjpeg2kDecodeStateCreate(handle, &decodeState); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to create decode state: %v", status)
	}
	defer C.nvjpeg2kDecodeStateDestroy(decodeState)

	// Configure output image structures
	outputImage := C.nvjpeg2kImage_t{
		pixel_type:     pixelType,
		num_components: C.uint32_t(numComponents),
		pixel_data:     (*unsafe.Pointer)(C.malloc(C.size_t(numComponents) * C.size_t(unsafe.Sizeof(unsafe.Pointer(nil))))),
		pitch_in_bytes: (*C.size_t)(C.malloc(C.size_t(numComponents) * C.size_t(unsafe.Sizeof(C.size_t(0))))),
	}
	defer C.free(unsafe.Pointer(outputImage.pixel_data))
	defer C.free(unsafe.Pointer(outputImage.pitch_in_bytes))

	// Array to store GPU memory pointers
	devicePtrs := make([]unsafe.Pointer, numComponents)
	defer func() {
		for _, ptr := range devicePtrs {
			C.cudaFree(ptr)
		}
	}()

	// Prepare memory for each component
	for i := 0; i < numComponents; i++ {
		if status := C.nvjpeg2kStreamGetImageComponentInfo(stream, &compInfo, C.uint32_t(i)); status != C.NVJPEG2K_STATUS_SUCCESS {
			return nil, fmt.Errorf("failed to get component info: %v", status)
		}

		bytesPerPixel := (int(compInfo.precision) + 7) / 8
		size := int(compInfo.component_width) * int(compInfo.component_height) * bytesPerPixel

		var devPtr unsafe.Pointer
		if status := C.cudaMalloc(&devPtr, C.size_t(size)); status != C.cudaSuccess {
			return nil, fmt.Errorf("cudaMalloc failed: %v", status)
		}
		devicePtrs[i] = devPtr

		ptrArray := (*[1<<30 - 1]*C.uchar)(unsafe.Pointer(outputImage.pixel_data))[:numComponents:numComponents]
		ptrArray[i] = (*C.uchar)(devPtr)

		pitchArray := (*[1<<30 - 1]C.size_t)(unsafe.Pointer(outputImage.pitch_in_bytes))[:numComponents:numComponents]
		pitchArray[i] = C.size_t(int(compInfo.component_width) * bytesPerPixel)
	}

	// Decode the image
	if status := C.nvjpeg2kDecodeImage(handle, decodeState, stream, decodeParams, &outputImage, nil); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("decode failed: %v", status)
	}

	// Create JP2Image for the result
	jp2Image := &jp2.JP2Image{
		Width:      int(imageInfo.image_width),
		Height:     int(imageInfo.image_height),
		Components: numComponents,
		Data:       make([][]float32, numComponents),
	}

	// Transfer data from GPU to CPU and convert to float32
	for i := 0; i < numComponents; i++ {
		if status := C.nvjpeg2kStreamGetImageComponentInfo(stream, &compInfo, C.uint32_t(i)); status != C.NVJPEG2K_STATUS_SUCCESS {
			return nil, fmt.Errorf("failed to get component info: %v", status)
		}

		width := int(compInfo.component_width)
		height := int(compInfo.component_height)
		precision := int(compInfo.precision)
		isSigned := compInfo.sgn != 0
		bytesPerPixel := (precision + 7) / 8
		totalSize := width * height * bytesPerPixel

		// Buffer for raw data
		rawData := make([]byte, totalSize)
		dataPtr := (*[1<<30 - 1]*C.uchar)(unsafe.Pointer(outputImage.pixel_data))[i]

		// Transfer from GPU to CPU
		if status := C.cudaMemcpy(
			unsafe.Pointer(&rawData[0]),
			unsafe.Pointer(dataPtr),
			C.size_t(totalSize),
			C.cudaMemcpyDeviceToHost,
		); status != C.cudaSuccess {
			return nil, fmt.Errorf("cudaMemcpy failed: %v", status)
		}

		// Allocate memory for normalized data
		jp2Image.Data[i] = make([]float32, width*height)

		// Normalization factor
		factor := float32(math.Pow(2, float64(precision)-1))

		// Convert by precision
		if precision <= 8 {
			for j := 0; j < width*height && j < len(rawData); j++ {
				if isSigned {
					jp2Image.Data[i][j] = float32(int8(rawData[j])) / factor
				} else {
					jp2Image.Data[i][j] = float32(rawData[j]) / factor
				}
			}
		} else if precision <= 16 {
			for j := 0; j < width*height && j*2+1 < len(rawData); j++ {
				value := uint16(rawData[j*2]) | (uint16(rawData[j*2+1]) << 8)
				if isSigned {
					jp2Image.Data[i][j] = float32(int16(value)) / factor
				} else {
					jp2Image.Data[i][j] = float32(value) / factor
				}
			}
		}
	}

	result.Metrics.DecodeTime = time.Since(startDecode)
	result.Metrics.NumTiles = int(imageInfo.num_tiles_x * imageInfo.num_tiles_y)
	result.Metrics.TotalTime = time.Since(startTotal)
	result.Image = jp2Image

	return result, nil
}
