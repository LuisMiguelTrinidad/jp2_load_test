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
	startTotal := time.Now()
	startInit := time.Now()

	// Inicialización de CUDA y nvjpeg2k
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

	tiempoInicializacion := time.Since(startInit)

	// Parseo del archivo
	startParse := time.Now()
	cFilePath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cFilePath))

	if status := C.nvjpeg2kStreamParseFile(handle, cFilePath, stream); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("failed to parse file: %v", status)
	}
	tiempoParseo := time.Since(startParse)

	// Obtención de información de la imagen
	startMetadata := time.Now()
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

	totalPixels := int64(0)
	totalBytes := int64(0)

	for i := 0; i < numComponents; i++ {
		var compInfo C.nvjpeg2kImageComponentInfo_t
		if status := C.nvjpeg2kStreamGetImageComponentInfo(stream, &compInfo, C.uint32_t(i)); status != C.NVJPEG2K_STATUS_SUCCESS {
			return nil, 0, 0, 0, fmt.Errorf("failed to get component info: %v", status)
		}

		components[i].Width = int(compInfo.component_width)
		components[i].Height = int(compInfo.component_height)
		components[i].Precision = int(compInfo.precision)
		components[i].Signed = compInfo.sgn != 0

		compPixels := int64(components[i].Width) * int64(components[i].Height)
		compBytes := compPixels * int64((components[i].Precision+7)/8)
		totalPixels += compPixels
		totalBytes += compBytes

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
	tiempoMetadata := time.Since(startMetadata)

	tiempoArchivo := time.Since(startTotal)
	startDecode := time.Now()

	// Configuración de parámetros de decodificación
	startParams := time.Now()
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
	tiempoParams := time.Since(startParams)

	// Configuración de estructuras para la imagen de salida
	startMemAlloc := time.Now()
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

	cudaAllocTimes := make([]time.Duration, numComponents)
	allocatedBytes := int64(0)

	for i := 0; i < numComponents; i++ {
		startCompAlloc := time.Now()
		comp := components[i]
		bytesPerPixel := (comp.Precision + 7) / 8
		size := comp.Width * comp.Height * bytesPerPixel
		allocatedBytes += int64(size)

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

		cudaAllocTimes[i] = time.Since(startCompAlloc)
	}
	tiempoMemAlloc := time.Since(startMemAlloc)

	// Decodificación de la imagen
	startGpuDecode := time.Now()
	if status := C.nvjpeg2kDecodeImage(handle, decodeState, stream, decodeParams, &outputImage, nil); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, 0, 0, 0, fmt.Errorf("decode failed: %v", status)
	}
	tiempoGpuDecode := time.Since(startGpuDecode)

	// Copia de datos de GPU a CPU
	startMemCopy := time.Now()
	cudaCopyTimes := make([]time.Duration, numComponents)

	for i := 0; i < numComponents; i++ {
		startCompCopy := time.Now()
		comp := &components[i]
		bytesPerPixel := (comp.Precision + 7) / 8
		bytesPerRow := comp.Width * bytesPerPixel
		totalSize := comp.Height * bytesPerRow

		comp.Data = make([]byte, totalSize)
		dataPtr := (*[1<<30 - 1]*C.uchar)(unsafe.Pointer(outputImage.pixel_data))[i]
		//pitch := int((*[1<<30 - 1]C.size_t)(unsafe.Pointer(outputImage.pitch_in_bytes))[i])

		if status := C.cudaMemcpy(
			unsafe.Pointer(&comp.Data[0]),
			unsafe.Pointer(dataPtr),
			C.size_t(totalSize),
			C.cudaMemcpyDeviceToHost,
		); status != C.cudaSuccess {
			return nil, 0, 0, 0, fmt.Errorf("cudaMemcpy failed: %v", status)
		}

		cudaCopyTimes[i] = time.Since(startCompCopy)
	}
	tiempoMemCopy := time.Since(startMemCopy)
	tiempoDecodif := time.Since(startDecode)
	tiempoTotal := time.Since(startTotal)

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

	// Estadísticas detalladas
	fmt.Printf("\n========== ESTADÍSTICAS DETALLADAS ==========\n")
	fmt.Printf("Tiempo total: %v\n", tiempoTotal)
	fmt.Printf("\n--- Desglose de tiempos ---\n")
	fmt.Printf("Inicialización nvjpeg2k: %v (%.2f%%)\n", tiempoInicializacion, porcentaje(tiempoInicializacion, tiempoTotal))
	fmt.Printf("Parseo del archivo: %v (%.2f%%)\n", tiempoParseo, porcentaje(tiempoParseo, tiempoTotal))
	fmt.Printf("Extracción metadata: %v (%.2f%%)\n", tiempoMetadata, porcentaje(tiempoMetadata, tiempoTotal))
	fmt.Printf("Configuración parámetros: %v (%.2f%%)\n", tiempoParams, porcentaje(tiempoParams, tiempoTotal))
	fmt.Printf("Asignación memoria GPU: %v (%.2f%%)\n", tiempoMemAlloc, porcentaje(tiempoMemAlloc, tiempoTotal))
	fmt.Printf("Decodificación GPU: %v (%.2f%%)\n", tiempoGpuDecode, porcentaje(tiempoGpuDecode, tiempoTotal))
	fmt.Printf("Transferencia GPU->CPU: %v (%.2f%%)\n", tiempoMemCopy, porcentaje(tiempoMemCopy, tiempoTotal))

	fmt.Printf("\n--- Estadísticas de componentes ---\n")
	for i := 0; i < numComponents; i++ {
		comp := components[i]
		compSize := int64(len(comp.Data))
		fmt.Printf("Componente %d (%dx%d, %d bits):\n", i, comp.Width, comp.Height, comp.Precision)
		fmt.Printf("  - Tiempo asignación GPU: %v\n", cudaAllocTimes[i])
		fmt.Printf("  - Tiempo copia GPU->CPU: %v\n", cudaCopyTimes[i])
		fmt.Printf("  - Velocidad transferencia: %.2f MB/s\n", float64(compSize)/(cudaCopyTimes[i].Seconds()*1024*1024))
	}

	fmt.Printf("\n--- Estadísticas de memoria ---\n")
	fmt.Printf("Memoria total asignada en GPU: %.2f MB\n", float64(allocatedBytes)/(1024*1024))
	fmt.Printf("Memoria total en componentes: %.2f MB\n", float64(totalBytes)/(1024*1024))
	fmt.Printf("Rendimiento total: %.2f MB/s\n", float64(totalBytes)/(tiempoTotal.Seconds()*1024*1024))
	fmt.Printf("Rendimiento decodificación: %.2f MB/s\n", float64(totalBytes)/(tiempoDecodif.Seconds()*1024*1024))
	fmt.Printf("===========================================\n")

	return &JP2Image{
		Components: components,
		Width:      int(imageInfo.image_width),
		Height:     int(imageInfo.image_height),
		ColorSpace: colorSpaceStr,
	}, tiempoArchivo, tiempoDecodif, numTiles, nil
}

// Función auxiliar para calcular porcentajes
func porcentaje(parte, total time.Duration) float64 {
	return float64(parte.Nanoseconds()) * 100 / float64(total.Nanoseconds())
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
