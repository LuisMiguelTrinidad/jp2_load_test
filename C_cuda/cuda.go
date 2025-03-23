package main

/*
#cgo LDFLAGS: -L/usr/local/cuda/lib64 -lcudart -lnvjpeg2k
#include "nvjpeg2k.h"
#include <cuda_runtime.h>
#include <stdlib.h>
#include <stdint.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"time"
	"unsafe"
)

type JP2Image struct {
	Components []Component
	Width      int
	Height     int
	ColorSpace string
}

type Component struct {
	Data      []byte
	Width     int
	Height    int
	Precision int
	Signed    bool
}

func readJP2Direct(filePath string, threads int) (*JP2Image, time.Duration, time.Duration, int, error) {
	startFile := time.Now()

	var handle C.nvjpeg2kHandle_t
	if status := C.nvjpeg2kCreateSimple(&handle); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("failed to create handle: %v", status)
	}
	defer C.nvjpeg2kDestroy(handle)

	var stream C.nvjpeg2kStream_t
	if status := C.nvjpeg2kStreamCreate(&stream); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("failed to create stream: %v", status)
	}
	defer C.nvjpeg2kStreamDestroy(stream)

	cFilePath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cFilePath))

	if status := C.nvjpeg2kStreamParseFile(handle, cFilePath, stream); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("failed to parse file: %v", status)
	}

	var imageInfo C.nvjpeg2kImageInfo_t
	if status := C.nvjpeg2kStreamGetImageInfo(stream, &imageInfo); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("failed to get image info: %v", status)
	}

	numComponents := int(imageInfo.num_components)
	components := make([]Component, numComponents)
	var pixelType C.nvjpeg2kImageType_t
	var colorSpace C.nvjpeg2kColorSpace_t

	if status := C.nvjpeg2kStreamGetColorSpace(stream, &colorSpace); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("failed to get color space: %v", status)
	}

	for i := 0; i < numComponents; i++ {
		var compInfo C.nvjpeg2kImageComponentInfo_t
		if status := C.nvjpeg2kStreamGetImageComponentInfo(stream, &compInfo, C.uint32_t(i)); status != C.NVJPEG2K_STATUS_SUCCESS {
			return nil, 0, 0, 0, fmt.Errorf("failed to get component info: %v", status)
		}

		components[i].Width = int(compInfo.component_width)
		components[i].Height = int(compInfo.component_height)
		components[i].Precision = int(compInfo.precision)
		components[i].Signed = compInfo.sgn != 0

		if i == 0 {
			switch {
			case compInfo.precision <= 8 && compInfo.sgn == 0:
				pixelType = C.NVJPEG2K_UINT8
			case compInfo.precision <= 16 && compInfo.sgn == 0:
				pixelType = C.NVJPEG2K_UINT16
			case compInfo.precision <= 16 && compInfo.sgn != 0:
				pixelType = C.NVJPEG2K_INT16
			default:
				return nil, 0, 0, 0, errors.New("unsupported component precision/signedness")
			}
		}
	}

	tiempoArchivo := time.Since(startFile)
	startDecode := time.Now()

	var decodeParams C.nvjpeg2kDecodeParams_t
	if status := C.nvjpeg2kDecodeParamsCreate(&decodeParams); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("failed to create decode params: %v", status)
	}
	defer C.nvjpeg2kDecodeParamsDestroy(decodeParams)

	C.nvjpeg2kDecodeParamsSetOutputFormat(decodeParams, C.NVJPEG2K_FORMAT_PLANAR)

	var decodeState C.nvjpeg2kDecodeState_t
	if status := C.nvjpeg2kDecodeStateCreate(handle, &decodeState); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("failed to create decode state: %v", status)
	}
	defer C.nvjpeg2kDecodeStateDestroy(decodeState)

	outputImage := C.nvjpeg2kImage_t{
		pixel_type:     pixelType,
		num_components: C.uint32_t(numComponents),
		pixel_data:     (*unsafe.Pointer)(C.malloc(C.size_t(numComponents) * C.size_t(unsafe.Sizeof(unsafe.Pointer(nil))))),
		pitch_in_bytes: (*C.size_t)(C.malloc(C.size_t(numComponents) * C.size_t(unsafe.Sizeof(C.size_t(0))))),
	}
	defer C.free(unsafe.Pointer(outputImage.pixel_data))
	defer C.free(unsafe.Pointer(outputImage.pitch_in_bytes))

	var devicePtrs []unsafe.Pointer
	defer func() {
		for _, ptr := range devicePtrs {
			C.cudaFree(ptr)
		}
	}()

	for i := 0; i < numComponents; i++ {
		comp := components[i]
		bytesPerPixel := (comp.Precision + 7) / 8
		size := comp.Width * comp.Height * bytesPerPixel

		var dataPtr *C.uchar
		var devPtr unsafe.Pointer
		if status := C.cudaMalloc(&devPtr, C.size_t(size)); status != C.cudaSuccess {
			return nil, 0, 0, 0, fmt.Errorf("cudaMalloc failed: %v", status)
		}
		dataPtr = (*C.uchar)(devPtr)
		devicePtrs = append(devicePtrs, devPtr)

		ptrArray := (*[1<<30 - 1]*C.uchar)(unsafe.Pointer(outputImage.pixel_data))[:numComponents:numComponents]
		ptrArray[i] = dataPtr

		pitchArray := (*[1<<30 - 1]C.size_t)(unsafe.Pointer(outputImage.pitch_in_bytes))[:numComponents:numComponents]
		pitchArray[i] = C.size_t(comp.Width * bytesPerPixel)
	}

	if status := C.nvjpeg2kDecodeImage(handle, decodeState, stream, decodeParams, &outputImage, nil); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("decode failed: %v", status)
	}

	for i := 0; i < numComponents; i++ {
		comp := &components[i]
		bytesPerPixel := (comp.Precision + 7) / 8
		bytesPerRow := comp.Width * bytesPerPixel
		totalSize := comp.Height * bytesPerRow

		comp.Data = make([]byte, totalSize)
		dataPtr := (*[1<<30 - 1]*C.uchar)(unsafe.Pointer(outputImage.pixel_data))[i]
		pitch := int((*[1<<30 - 1]C.size_t)(unsafe.Pointer(outputImage.pitch_in_bytes))[i])
		fmt.Printf("Component %d has pitch: %d bytes\n", i, pitch)

		if status := C.cudaMemcpy(
			unsafe.Pointer(&comp.Data[0]),
			unsafe.Pointer(dataPtr),
			C.size_t(totalSize),
			C.cudaMemcpyDeviceToHost,
		); status != C.cudaSuccess {
			return nil, 0, 0, 0, fmt.Errorf("cudaMemcpy failed: %v", status)
		}
	}

	tiempoDecodif := time.Since(startDecode)
	numTiles := int(imageInfo.num_tiles_x * imageInfo.num_tiles_y)

	colorSpaceStr := "Unknown"
	switch colorSpace {
	case C.NVJPEG2K_COLORSPACE_SRGB:
		colorSpaceStr = "sRGB"
	case C.NVJPEG2K_COLORSPACE_GRAY:
		colorSpaceStr = "Grayscale"
	case C.NVJPEG2K_COLORSPACE_SYCC:
		colorSpaceStr = "SYCC"
	}

	return &JP2Image{
		Components: components,
		Width:      int(imageInfo.image_width),
		Height:     int(imageInfo.image_height),
		ColorSpace: colorSpaceStr,
	}, tiempoArchivo, tiempoDecodif, numTiles, nil
}

func main() {
	// Definir la ruta del archivo JP2
	filePath := "../input_images/B04_10m.jp2"
	threads := 4 // Número de hilos (aunque no se usa actualmente en readJP2Direct)

	fmt.Printf("Cargando imagen JP2: %s\n", filePath)

	// Cargar y decodificar la imagen
	image, tiempoArchivo, tiempoDecodif, numTiles, err := readJP2Direct(filePath, threads)
	if err != nil {
		fmt.Printf("Error al cargar la imagen: %v\n", err)
		return
	}

	// Mostrar información sobre la imagen
	fmt.Printf("\nInformación de la imagen JP2:\n")
	fmt.Printf("Dimensiones: %d x %d\n", image.Width, image.Height)
	fmt.Printf("Espacio de color: %s\n", image.ColorSpace)
	fmt.Printf("Número de componentes: %d\n", len(image.Components))
	fmt.Printf("Número de tiles: %d\n", numTiles)

	// Mostrar detalles de cada componente
	for i, comp := range image.Components {
		fmt.Printf("\nComponente %d:\n", i)
		fmt.Printf("  Dimensiones: %d x %d\n", comp.Width, comp.Height)
		fmt.Printf("  Precisión: %d bits\n", comp.Precision)
		fmt.Printf("  Con signo: %v\n", comp.Signed)
		fmt.Printf("  Tamaño de datos: %d bytes\n", len(comp.Data))
	}

	// Mostrar tiempos de procesamiento
	fmt.Printf("\nTiempos de procesamiento:\n")
	fmt.Printf("Análisis del archivo: %v\n", tiempoArchivo)
	fmt.Printf("Decodificación: %v\n", tiempoDecodif)
	fmt.Printf("Tiempo total: %v\n", tiempoArchivo+tiempoDecodif)

	fmt.Println("\nLa imagen se cargó correctamente.")
}
