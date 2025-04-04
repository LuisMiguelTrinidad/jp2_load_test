package gpu

import (
	"fmt"
	"image"
	"os"
	"time"
	"unsafe"
)

// #cgo LDFLAGS: -L/usr/local/cuda/lib64 -lcudart -lnvjpeg2k
// #include "nvjpeg2k.h"
// #include <cuda_runtime.h>
// #include <stdlib.h>
import "C"

// Writer implements jp2.Writer for GPU-based encoding
type Writer struct {
	outputPath string
}

// NewWriter creates a new GPU-based JP2 writer
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
	outputName := outputPath

	// Initialize nvjpeg2k encoder
	var encoder C.nvjpeg2kEncoder_t
	if status := C.nvjpeg2kEncoderCreateSimple(&encoder); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to create encoder: %v", status)
	}
	defer C.nvjpeg2kEncoderDestroy(encoder)

	var encodeState C.nvjpeg2kEncodeState_t
	if status := C.nvjpeg2kEncodeStateCreate(encoder, &encodeState); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to create encode state: %v", status)
	}
	defer C.nvjpeg2kEncodeStateDestroy(encodeState)

	var encodeParams C.nvjpeg2kEncodeParams_t
	if status := C.nvjpeg2kEncodeParamsCreate(&encodeParams); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to create encode params: %v", status)
	}
	defer C.nvjpeg2kEncodeParamsDestroy(encodeParams)

	// Configure input format for RGBA data (interleaved)
	if status := C.nvjpeg2kEncodeParamsSetInputFormat(encodeParams, C.NVJPEG2K_FORMAT_INTERLEAVED); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to set input format: %v", status)
	}

	// Configure quality (using PSNR - higher number = better quality)
	if status := C.nvjpeg2kEncodeParamsSetQuality(encodeParams, 40.0); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to set quality: %v", status)
	}

	// Image dimensions
	imgBounds := img.Bounds()
	width := imgBounds.Dx()
	height := imgBounds.Dy()

	// Configure JP2 encoding parameters
	compInfoSize := C.size_t(unsafe.Sizeof(C.nvjpeg2kImageComponentInfo_t{})) * 4 // 4 RGBA components
	compInfoPtr := C.malloc(compInfoSize)
	defer C.free(compInfoPtr)

	// Configure encoding configuration structure
	encodeConfig := C.nvjpeg2kEncodeConfig_t{
		stream_type:     C.NVJPEG2K_STREAM_JP2,      // JP2 format
		color_space:     C.NVJPEG2K_COLORSPACE_SRGB, // RGB Color
		image_width:     C.uint32_t(width),
		image_height:    C.uint32_t(height),
		num_components:  4, // RGBA
		image_comp_info: (*C.nvjpeg2kImageComponentInfo_t)(compInfoPtr),
		prog_order:      C.NVJPEG2K_LRCP, // Standard progression order
		num_layers:      1,
		mct_mode:        1,  // Color transform enabled
		num_resolutions: 6,  // Number of resolution levels
		code_block_w:    64, // Code block width
		code_block_h:    64, // Code block height
		irreversible:    1,  // Irreversible transform for better compression
	}

	// Configure each component
	for i := 0; i < 4; i++ {
		compInfo := (*C.nvjpeg2kImageComponentInfo_t)(unsafe.Pointer(
			uintptr(unsafe.Pointer(encodeConfig.image_comp_info)) + uintptr(i)*unsafe.Sizeof(C.nvjpeg2kImageComponentInfo_t{}),
		))
		compInfo.component_width = C.uint32_t(width)
		compInfo.component_height = C.uint32_t(height)
		compInfo.precision = 8 // 8 bits per component
		compInfo.sgn = 0       // Unsigned
	}

	// Apply encoding configuration
	if status := C.nvjpeg2kEncodeParamsSetEncodeConfig(encodeParams, &encodeConfig); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to set encode config: %v", status)
	}

	// Prepare image for GPU encoding
	outputImage := C.nvjpeg2kImage_t{
		pixel_type:     C.NVJPEG2K_UINT8,
		num_components: 4, // RGBA
		pixel_data:     (*unsafe.Pointer)(C.malloc(C.size_t(4) * C.size_t(unsafe.Sizeof(unsafe.Pointer(nil))))),
		pitch_in_bytes: (*C.size_t)(C.malloc(C.size_t(4) * C.size_t(unsafe.Sizeof(C.size_t(0))))),
	}
	defer C.free(unsafe.Pointer(outputImage.pixel_data))
	defer C.free(unsafe.Pointer(outputImage.pitch_in_bytes))

	// Allocate GPU memory for image data
	var devicePtr unsafe.Pointer
	totalSize := width * height * 4 // 4 bytes per RGBA pixel

	if status := C.cudaMalloc(&devicePtr, C.size_t(totalSize)); status != C.cudaSuccess {
		return 0, fmt.Errorf("cudaMalloc failed: %v", status)
	}
	defer C.cudaFree(devicePtr)

	// Transfer image data from CPU to GPU
	if status := C.cudaMemcpy(
		devicePtr,
		unsafe.Pointer(&img.Pix[0]),
		C.size_t(totalSize),
		C.cudaMemcpyHostToDevice,
	); status != C.cudaSuccess {
		return 0, fmt.Errorf("cudaMemcpy to device failed: %v", status)
	}

	// Configure pointer and pitch for the image
	ptrArray := (*[4]*C.uchar)(unsafe.Pointer(outputImage.pixel_data))
	ptrArray[0] = (*C.uchar)(devicePtr)

	pitchArray := (*[4]C.size_t)(unsafe.Pointer(outputImage.pitch_in_bytes))
	pitchArray[0] = C.size_t(width * 4) // 4 bytes per pixel (RGBA)

	// Encode the image
	if status := C.nvjpeg2kEncode(encoder, encodeState, encodeParams, &outputImage, nil); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("encode failed: %v", status)
	}

	// Get encoded bitstream size
	var length C.size_t
	if status := C.nvjpeg2kEncodeRetrieveBitstream(encoder, encodeState, nil, &length, nil); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to get bitstream size: %v", status)
	}

	// Allocate memory for the bitstream
	bitstreamData := make([]byte, int(length))

	// Retrieve encoded data
	if status := C.nvjpeg2kEncodeRetrieveBitstream(
		encoder,
		encodeState,
		(*C.uchar)(unsafe.Pointer(&bitstreamData[0])),
		&length,
		nil,
	); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to retrieve bitstream: %v", status)
	}

	// Save to file
	if err := os.WriteFile(outputName, bitstreamData[:int(length)], 0644); err != nil {
		return 0, fmt.Errorf("failed to write file: %v", err)
	}

	saveTime := time.Since(startSave)
	return saveTime, nil
}
