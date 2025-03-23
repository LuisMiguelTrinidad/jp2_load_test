package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// #cgo LDFLAGS: -L/usr/local/cuda/lib64 -lcudart -lnvjpeg2k
// #cgo CFLAGS: -I/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/include
// #cgo LDFLAGS: -L/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/lib -lopenjp2
// #include "nvjpeg2k.h"
// #include <cuda_runtime.h>
// #include <openjpeg-2.5/openjpeg.h>
// #include <stdlib.h>
// #include <string.h>
// #include <stdint.h>
//
// // Callback functions for OpenJPEG
// void error_callback(const char *msg, void *client_data) {
//     fprintf(stderr, "[ERROR] %s", msg);
// }
//
// void warning_callback(const char *msg, void *client_data) {
//     fprintf(stderr, "[WARNING] %s", msg);
// }
//
// void info_callback(const char *msg, void *client_data) {
// //    fprintf(stdout, "[INFO] %s", msg);
// }
import "C"

// Colores para el gradiente NDVI
var ndviGradientPoints = []struct {
	Value float64
	Color color.RGBA
}{
	{-1.0, color.RGBA{0, 0, 128, 255}},    // Azul oscuro (agua/sombras)
	{-0.2, color.RGBA{65, 105, 225, 255}}, // Azul medio
	{0.0, color.RGBA{255, 0, 0, 255}},     // Rojo (suelo/áreas urbanas)
	{0.5, color.RGBA{255, 255, 0, 255}},   // Amarillo (vegetación dispersa)
	{1.0, color.RGBA{0, 128, 0, 255}},     // Verde (vegetación densa)
}

func printMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("Alloc = %v MiB", m.Alloc/1024/1024)
	fmt.Printf("\tTotalAlloc = %v MiB", m.TotalAlloc/1024/1024)
	fmt.Printf("\tSys = %v MiB", m.Sys/1024/1024)
	fmt.Printf("\tNumGC = %v MiB", m.NumGC)
	fmt.Printf("\tHeapAlloc = %v MiB", m.HeapAlloc/1024/1024)
	fmt.Printf("\tHeapSys = %v MiB", m.HeapSys/1024/1024)
	fmt.Printf("\tHeapIdle = %v MiB", m.HeapIdle/1024/1024)
	fmt.Printf("\tHeapInuse = %v MiB", m.HeapInuse/1024/1024)
	fmt.Printf("\tHeapReleased = %v MiB", m.HeapReleased/1024/1024)
	fmt.Printf("\tHeapObjects = %v MiB\n", m.HeapObjects)
}

type Metricas struct {
	Resolucion       string
	TipoProcesador   string // "CPU" o "GPU"
	NumHilos         int    // Número de hilos utilizados (para CPU)
	TiempoTotal      time.Duration
	TiempoLectura    time.Duration
	TiempoNDVI       time.Duration
	TiempoColor      time.Duration
	TiempoGuardado   time.Duration
	TiempoArchivoNIR time.Duration
	TiempoDecodifNIR time.Duration
	TiempoArchivoRED time.Duration
	TiempoDecodifRED time.Duration
	Pixeles          int
	PixelesSinDatos  int
	TamanoImagen     int64
	NDVIMin          float64
	NDVIMax          float64
	NDVIPromedio     float64
	NumTilesNIR      int
	NumTilesRED      int
}

// MetricasLectura contiene las métricas asociadas a la lectura de un archivo JP2
type MetricasLectura struct {
	TiempoArchivo time.Duration
	TiempoDecodif time.Duration
	NumTiles      int
	TiempoTotal   time.Duration
}

// ResultadoBanda contiene una imagen JP2 y sus métricas de lectura
type ResultadoBanda struct {
	Imagen   *JP2Image
	Metricas MetricasLectura
}

// MetricasNDVI contiene las métricas del cálculo de NDVI
type MetricasNDVI struct {
	Tiempo          time.Duration
	PixelesTotales  int
	PixelesSinDatos int
	Min             float64
	Max             float64
	Promedio        float64
}

// MetricasColor contiene las métricas de la colorización
type MetricasColor struct {
	Tiempo       time.Duration
	TamanoImagen int64
}

// JP2Image representa una imagen JPEG2000 decodificada
type JP2Image struct {
	Width, Height int
	Components    int
	Data          [][]float32 // Un slice por componente
}

func (img *JP2Image) Free() {
	if img == nil {
		return
	}
	for i := range img.Data {
		// Simplemente establecer a nil es suficiente
		// para que el recolector de basura de Go libere la memoria
		img.Data[i] = nil
	}
	runtime.SetFinalizer(img, nil)
}

// Free libera la memoria utilizada por ResultadoBanda
func (rb *ResultadoBanda) Free() {
	if rb == nil {
		return
	}

	// Liberar la imagen JP2
	if rb.Imagen != nil {
		rb.Imagen.Free()
		rb.Imagen = nil
	}
}

// Buscar índices de gradiente eficientemente
func ndviColorOptimized(ndviValue float64) color.RGBA {
	if ndviValue <= -1.0 {
		return ndviGradientPoints[0].Color
	}
	if ndviValue >= 1.0 {
		return ndviGradientPoints[len(ndviGradientPoints)-1].Color
	}

	// Búsqueda binaria para el índice
	var idx int
	lo, hi := 0, len(ndviGradientPoints)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		if ndviGradientPoints[mid].Value > ndviValue {
			hi = mid - 1
		} else {
			lo = mid + 1
			idx = mid
		}
	}

	// Caso especial para el último punto
	if idx >= len(ndviGradientPoints)-1 {
		return ndviGradientPoints[len(ndviGradientPoints)-1].Color
	}

	// Interpolación eficiente
	p1, p2 := ndviGradientPoints[idx], ndviGradientPoints[idx+1]
	t := (ndviValue - p1.Value) / (p2.Value - p1.Value)

	r := uint8(float64(p1.Color.R) + t*float64(p2.Color.R-p1.Color.R))
	g := uint8(float64(p1.Color.G) + t*float64(p2.Color.G-p1.Color.G))
	b := uint8(float64(p1.Color.B) + t*float64(p2.Color.B-p1.Color.B))

	return color.RGBA{r, g, b, 255}
}

// ReadCPU lee una imagen JP2 usando la CPU
func ReadCPU(filePath string, threads int) (*ResultadoBanda, error) {
	resultado := &ResultadoBanda{
		Metricas: MetricasLectura{},
	}

	inicioTotal := time.Now()

	// Medir tiempo de apertura del archivo
	inicioArchivo := time.Now()

	// Convertir string a C string
	cFilePath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cFilePath))

	// Configurar el stream
	stream := C.opj_stream_create_default_file_stream(cFilePath, 1)
	if stream == nil {
		return nil, fmt.Errorf("no se pudo abrir el archivo: %s", filePath)
	}
	defer C.opj_stream_destroy(stream)

	resultado.Metricas.TiempoArchivo = time.Since(inicioArchivo)

	// Medir tiempo de decodificación
	inicioDecodif := time.Now()

	// Crear el codec
	codec := C.opj_create_decompress(C.OPJ_CODEC_JP2)
	if codec == nil {
		return nil, fmt.Errorf("no se pudo crear el codec JP2")
	}
	defer C.opj_destroy_codec(codec)

	// Configurar los callbacks
	C.opj_set_error_handler(codec, (C.opj_msg_callback)(C.error_callback), nil)
	C.opj_set_warning_handler(codec, (C.opj_msg_callback)(C.warning_callback), nil)
	C.opj_set_info_handler(codec, (C.opj_msg_callback)(C.info_callback), nil)

	// Configurar los parámetros
	parameters := C.opj_dparameters_t{}
	C.opj_set_default_decoder_parameters(&parameters)

	if C.opj_setup_decoder(codec, &parameters) == C.OPJ_FALSE {
		return nil, fmt.Errorf("error al configurar el decodificador")
	}

	// Configurar el número de hilos si se especifica más de 1
	if threads > 1 {
		if C.opj_codec_set_threads(codec, C.int(threads)) == C.OPJ_FALSE {
			fmt.Printf("Advertencia: No se pudo configurar %d hilos, continuando con configuración por defecto\n", threads)
		} else {
			fmt.Printf("Decodificando con %d hilos\n", threads)
		}
	}

	// Leer el header
	var image *C.opj_image_t
	if C.opj_read_header(stream, codec, &image) == C.OPJ_FALSE {
		C.opj_destroy_codec(codec)
		C.opj_stream_destroy(stream)
		return nil, fmt.Errorf("error al leer el header de la imagen")
	}

	// Obtener información sobre los tiles después de leer el encabezado
	var numTiles int = 1 // Valor por defecto si no se puede obtener la info
	cstrInfo := C.opj_get_cstr_info(codec)
	if cstrInfo != nil {
		numTiles = int(cstrInfo.tw * cstrInfo.th)
		C.opj_destroy_cstr_info(&cstrInfo)
	}

	// Decodificar la imagen
	if C.opj_decode(codec, stream, image) == C.OPJ_FALSE {
		C.opj_image_destroy(image)
		return nil, fmt.Errorf("error al decodificar la imagen")
	}

	// Convertir la imagen de OpenJPEG a nuestro formato
	jp2Image := &JP2Image{
		Width:      int(image.x1 - image.x0),
		Height:     int(image.y1 - image.y0),
		Components: int(image.numcomps),
		Data:       make([][]float32, int(image.numcomps)),
	}

	// Extraer datos de cada componente
	for i := range jp2Image.Components {
		comp := *(*C.opj_image_comp_t)(unsafe.Pointer(
			uintptr(unsafe.Pointer(image.comps)) + uintptr(i)*unsafe.Sizeof(C.opj_image_comp_t{}),
		))

		// Asignamos memoria para los datos de esta componente
		jp2Image.Data[i] = make([]float32, jp2Image.Width*jp2Image.Height)

		// Obtenemos el valor del componente una sola vez fuera del bucle
		compData := (*[1 << 30]C.int)(unsafe.Pointer(comp.data))[: jp2Image.Width*jp2Image.Height : jp2Image.Width*jp2Image.Height]

		// Copiamos los datos normalizados
		factor := math.Pow(2, float64(comp.prec)-1)
		for j := range jp2Image.Width * jp2Image.Height {
			// Normalizamos a [0,1]
			jp2Image.Data[i][j] = float32(float64(compData[j]) / factor)
		}
	}

	// Liberar memoria de la imagen
	C.opj_image_destroy(image)

	resultado.Metricas.TiempoDecodif = time.Since(inicioDecodif)
	resultado.Metricas.NumTiles = numTiles
	resultado.Metricas.TiempoTotal = time.Since(inicioTotal)
	resultado.Imagen = jp2Image

	return resultado, nil
}

// ReadGPU lee una imagen JP2 usando la GPU
// El parámetro 'threads' se mantiene por compatibilidad con ReadCPU pero no tiene efecto en la GPU
func ReadGPU(filePath string, threads int) (*ResultadoBanda, error) {
	resultado := &ResultadoBanda{
		Metricas: MetricasLectura{},
	}

	inicioTotal := time.Now()

	// Inicialización de CUDA y nvjpeg2k
	var handle C.nvjpeg2kHandle_t
	if status := C.nvjpeg2kCreateSimple(&handle); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to create handle: %v", status)
	}
	defer C.nvjpeg2kDestroy(handle)

	var stream C.nvjpeg2kStream_t
	if status := C.nvjpeg2kStreamCreate(&stream); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to create stream: %v", status)
	}
	defer C.nvjpeg2kStreamDestroy(stream)

	// Parseo del archivo
	inicioArchivo := time.Now()
	cFilePath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cFilePath))

	if status := C.nvjpeg2kStreamParseFile(handle, cFilePath, stream); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to parse file: %v", status)
	}

	// Obtención de información de la imagen
	var imageInfo C.nvjpeg2kImageInfo_t
	if status := C.nvjpeg2kStreamGetImageInfo(stream, &imageInfo); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("failed to get image info: %v", status)
	}

	resultado.Metricas.TiempoArchivo = time.Since(inicioArchivo)

	// Decodificación
	inicioDecodif := time.Now()

	numComponents := int(imageInfo.num_components)
	var pixelType C.nvjpeg2kImageType_t

	// Determinar tipo de píxel basado en el primer componente
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
		return nil, errors.New("precisión/signo de componente no soportado")
	}

	// Configuración de parámetros de decodificación
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

	// Configuración de estructuras para la imagen de salida
	outputImage := C.nvjpeg2kImage_t{
		pixel_type:     pixelType,
		num_components: C.uint32_t(numComponents),
		pixel_data:     (*unsafe.Pointer)(C.malloc(C.size_t(numComponents) * C.size_t(unsafe.Sizeof(unsafe.Pointer(nil))))),
		pitch_in_bytes: (*C.size_t)(C.malloc(C.size_t(numComponents) * C.size_t(unsafe.Sizeof(C.size_t(0))))),
	}
	defer C.free(unsafe.Pointer(outputImage.pixel_data))
	defer C.free(unsafe.Pointer(outputImage.pitch_in_bytes))

	// Array para guardar punteros a memoria en GPU
	devicePtrs := make([]unsafe.Pointer, numComponents)
	defer func() {
		for _, ptr := range devicePtrs {
			C.cudaFree(ptr)
		}
	}()

	// Preparar memoria para cada componente
	for i := range numComponents {
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

	// Decodificar la imagen
	if status := C.nvjpeg2kDecodeImage(handle, decodeState, stream, decodeParams, &outputImage, nil); status != C.NVJPEG2K_STATUS_SUCCESS {
		return nil, fmt.Errorf("decode failed: %v", status)
	}

	// Crear la estructura JP2Image para el resultado
	jp2Image := &JP2Image{
		Width:      int(imageInfo.image_width),
		Height:     int(imageInfo.image_height),
		Components: numComponents,
		Data:       make([][]float32, numComponents),
	}

	// Transferir datos de GPU a CPU y convertir a float32
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

		// Buffer para los datos crudos
		rawData := make([]byte, totalSize)
		dataPtr := (*[1<<30 - 1]*C.uchar)(unsafe.Pointer(outputImage.pixel_data))[i]

		// Transferir de GPU a CPU
		if status := C.cudaMemcpy(
			unsafe.Pointer(&rawData[0]),
			unsafe.Pointer(dataPtr),
			C.size_t(totalSize),
			C.cudaMemcpyDeviceToHost,
		); status != C.cudaSuccess {
			return nil, fmt.Errorf("cudaMemcpy failed: %v", status)
		}

		// Asignar memoria para los datos normalizados
		jp2Image.Data[i] = make([]float32, width*height)

		// Factor para normalizar según precisión
		factor := float32(math.Pow(2, float64(precision)-1))

		// Convertir según precisión
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

	resultado.Metricas.TiempoDecodif = time.Since(inicioDecodif)
	resultado.Metricas.NumTiles = int(imageInfo.num_tiles_x * imageInfo.num_tiles_y)
	resultado.Metricas.TiempoTotal = time.Since(inicioTotal)
	resultado.Imagen = jp2Image

	return resultado, nil
}

// StoreCPU guarda una imagen NDVI como JPEG2000 usando la CPU con OpenJPEG
func StoreCPU(ndviColorImg *image.RGBA, resolucion string, threads int) (time.Duration, error) {
	inicioGuardado := time.Now()
	// Crear directorio si no existe
	if err := os.MkdirAll("./go_jp2_direct", 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %v", err)
	}
	// Nombre del archivo de salida
	nombreColor := fmt.Sprintf("./go_jp2_direct/ndvi_%s_color.jp2", resolucion)
	cfilename := C.CString(nombreColor)
	defer C.free(unsafe.Pointer(cfilename))

	// Obtener dimensiones de la imagen
	bounds := ndviColorImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pix := ndviColorImg.Pix

	// Configurar parámetros de componentes (RGBA)
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

	// Crear imagen OpenJPEG
	cimage := C.opj_image_create(4, &cmptparm[0], C.OPJ_CLRSPC_SRGB)
	if cimage == nil {
		return 0, errors.New("failed to create OpenJPEG image")
	}
	defer C.opj_image_destroy(cimage)

	// Configurar coordenadas de origen
	cimage.x0 = 0
	cimage.y0 = 0
	cimage.x1 = C.uint(width)
	cimage.y1 = C.uint(height)

	// Copiar datos RGBA a componentes OpenJPEG
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
			for j := range pixelCount {
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
	// Configurar parámetros del encoder
	var parameters C.opj_cparameters_t
	C.opj_set_default_encoder_parameters(&parameters)

	// Limitar el número de hilos para imágenes grandes para evitar
	// consumo excesivo de memoria

	// Resto de la configuración igual...
	parameters.tcp_numlayers = 1
	parameters.tcp_rates[0] = 0
	parameters.irreversible = 0
	parameters.numresolution = 6
	parameters.cp_disto_alloc = 1

	// Crear codec JP2
	codec := C.opj_create_compress(C.OPJ_CODEC_JP2)
	if codec == nil {
		return 0, errors.New("failed to create JP2 codec")
	}
	defer C.opj_destroy_codec(codec)

	// Configurar hilos con el límite aplicado

	return time.Since(inicioGuardado), nil
}

// StoreGPU guarda una imagen NDVI como JPEG2000 usando la GPU
func StoreGPU(ndviColorImg *image.RGBA, resolucion string) (time.Duration, error) {
	inicioGuardado := time.Now()

	// Crear directorio si no existe
	os.MkdirAll("./go_jp2_direct", 0755)

	// Nombre del archivo de salida (usamos .jp2 para JPEG2000)
	nombreColor := fmt.Sprintf("./go_jp2_direct/ndvi_%s_color.jp2", resolucion)

	// Inicialización del encoder nvjpeg2k
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

	// Configurar formato de entrada para datos RGBA (interleaved)
	if status := C.nvjpeg2kEncodeParamsSetInputFormat(encodeParams, C.NVJPEG2K_FORMAT_INTERLEAVED); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to set input format: %v", status)
	}

	// Configurar calidad (usando PSNR - mayor número = mejor calidad)
	if status := C.nvjpeg2kEncodeParamsSetQuality(encodeParams, 40.0); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to set quality: %v", status)
	}

	// Dimensiones de la imagen
	imgBounds := ndviColorImg.Bounds()
	width := imgBounds.Dx()
	height := imgBounds.Dy()

	// Configurar los parámetros de codificación JP2
	compInfoSize := C.size_t(unsafe.Sizeof(C.nvjpeg2kImageComponentInfo_t{})) * 4 // 4 componentes RGBA
	compInfoPtr := C.malloc(compInfoSize)
	defer C.free(compInfoPtr)

	// Configurar la estructura de configuración de codificación
	encodeConfig := C.nvjpeg2kEncodeConfig_t{
		stream_type:     C.NVJPEG2K_STREAM_JP2,      // Formato JP2
		color_space:     C.NVJPEG2K_COLORSPACE_SRGB, // Color RGB
		image_width:     C.uint32_t(width),
		image_height:    C.uint32_t(height),
		num_components:  4, // RGBA
		image_comp_info: (*C.nvjpeg2kImageComponentInfo_t)(compInfoPtr),
		prog_order:      C.NVJPEG2K_LRCP, // Orden de progresión estándar
		num_layers:      1,
		mct_mode:        1,  // Transformación de color activada
		num_resolutions: 6,  // Número de niveles de resolución
		code_block_w:    64, // Ancho de bloque de código
		code_block_h:    64, // Alto de bloque de código
		irreversible:    1,  // Transformación irreversible para mejor compresión
	}

	// Configurar cada componente
	for i := 0; i < 4; i++ {
		compInfo := (*C.nvjpeg2kImageComponentInfo_t)(unsafe.Pointer(
			uintptr(unsafe.Pointer(encodeConfig.image_comp_info)) + uintptr(i)*unsafe.Sizeof(C.nvjpeg2kImageComponentInfo_t{}),
		))
		compInfo.component_width = C.uint32_t(width)
		compInfo.component_height = C.uint32_t(height)
		compInfo.precision = 8 // 8 bits por componente
		compInfo.sgn = 0       // Sin signo
	}

	// Aplicar la configuración de codificación
	if status := C.nvjpeg2kEncodeParamsSetEncodeConfig(encodeParams, &encodeConfig); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to set encode config: %v", status)
	}

	// Preparar la imagen para codificación en GPU
	outputImage := C.nvjpeg2kImage_t{
		pixel_type:     C.NVJPEG2K_UINT8,
		num_components: 4, // RGBA
		pixel_data:     (*unsafe.Pointer)(C.malloc(C.size_t(4) * C.size_t(unsafe.Sizeof(unsafe.Pointer(nil))))),
		pitch_in_bytes: (*C.size_t)(C.malloc(C.size_t(4) * C.size_t(unsafe.Sizeof(C.size_t(0))))),
	}
	defer C.free(unsafe.Pointer(outputImage.pixel_data))
	defer C.free(unsafe.Pointer(outputImage.pitch_in_bytes))

	// Asignar memoria en GPU para los datos de la imagen
	var devicePtr unsafe.Pointer
	totalSize := width * height * 4 // 4 bytes por pixel RGBA

	if status := C.cudaMalloc(&devicePtr, C.size_t(totalSize)); status != C.cudaSuccess {
		return 0, fmt.Errorf("cudaMalloc failed: %v", status)
	}
	defer C.cudaFree(devicePtr)

	// Transferir datos de la imagen CPU a GPU
	if status := C.cudaMemcpy(
		devicePtr,
		unsafe.Pointer(&ndviColorImg.Pix[0]),
		C.size_t(totalSize),
		C.cudaMemcpyHostToDevice,
	); status != C.cudaSuccess {
		return 0, fmt.Errorf("cudaMemcpy to device failed: %v", status)
	}

	// Configurar puntero y pitch para la imagen
	ptrArray := (*[4]*C.uchar)(unsafe.Pointer(outputImage.pixel_data))
	ptrArray[0] = (*C.uchar)(devicePtr)

	pitchArray := (*[4]C.size_t)(unsafe.Pointer(outputImage.pitch_in_bytes))
	pitchArray[0] = C.size_t(width * 4) // 4 bytes por pixel (RGBA)

	// Codificar la imagen
	if status := C.nvjpeg2kEncode(encoder, encodeState, encodeParams, &outputImage, nil); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("encode failed: %v", status)
	}

	// Obtener el tamaño del bitstream codificado
	var length C.size_t
	if status := C.nvjpeg2kEncodeRetrieveBitstream(encoder, encodeState, nil, &length, nil); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to get bitstream size: %v", status)
	}

	// Asignar memoria para el bitstream
	bitstreamData := make([]byte, int(length))

	// Recuperar los datos codificados
	if status := C.nvjpeg2kEncodeRetrieveBitstream(
		encoder,
		encodeState,
		(*C.uchar)(unsafe.Pointer(&bitstreamData[0])),
		&length,
		nil,
	); status != C.NVJPEG2K_STATUS_SUCCESS {
		return 0, fmt.Errorf("failed to retrieve bitstream: %v", status)
	}

	// Guardar a archivo
	if err := os.WriteFile(nombreColor, bitstreamData[:int(length)], 0644); err != nil {
		return 0, fmt.Errorf("failed to write file: %v", err)
	}

	tiempoGuardado := time.Since(inicioGuardado)
	return tiempoGuardado, nil
}

// CalculateNDVI calcula el índice NDVI y genera una imagen colorizada
func CalculateNDVI(nirResultado, redResultado *ResultadoBanda, numCPUs int) (*MetricasNDVI, *MetricasColor, *image.RGBA, error) {
	metricasNDVI := &MetricasNDVI{
		Min: math.MaxFloat64,
		Max: -math.MaxFloat64,
	}

	metricasColor := &MetricasColor{}

	// Verificar que las dimensiones sean iguales
	if nirResultado.Imagen.Width != redResultado.Imagen.Width || nirResultado.Imagen.Height != redResultado.Imagen.Height {
		return nil, nil, nil, fmt.Errorf("las dimensiones de las imágenes NIR y RED no coinciden")
	}

	// Obtener dimensiones
	width := nirResultado.Imagen.Width
	height := redResultado.Imagen.Height
	pixelCount := width * height
	metricasNDVI.PixelesTotales = pixelCount

	// Calcular NDVI en paralelo
	numWorkers := numCPUs
	ndviData := make([]float64, pixelCount)

	inicioNDVI := time.Now()

	chunkSize := (pixelCount + numWorkers - 1) / numWorkers
	minVals := make([]float64, numWorkers)
	maxVals := make([]float64, numWorkers)
	sums := make([]float64, numWorkers)
	pixelesSinDatos := make([]int, numWorkers)

	var wgNDVI sync.WaitGroup
	wgNDVI.Add(numWorkers)

	nirData := nirResultado.Imagen.Data[0]
	redData := redResultado.Imagen.Data[0]

	for w := range numWorkers {
		start := w * chunkSize
		end := min(start+chunkSize, pixelCount)

		go func(worker, start, end int) {
			defer wgNDVI.Done()
			localMin := math.MaxFloat64
			localMax := -math.MaxFloat64
			localSum := 0.0
			localNoData := 0

			for i := start; i < end; i++ {
				nirVal := float64(nirData[i])
				redVal := float64(redData[i])

				sum := nirVal + redVal
				ndviValue := 0.0
				if sum > 0 {
					ndviValue = (nirVal - redVal) / sum
				} else {
					// Contar como píxel sin datos
					localNoData++
				}
				ndviData[i] = ndviValue

				if ndviValue < localMin {
					localMin = ndviValue
				}
				if ndviValue > localMax {
					localMax = ndviValue
				}
				localSum += ndviValue
			}

			minVals[worker] = localMin
			maxVals[worker] = localMax
			sums[worker] = localSum
			pixelesSinDatos[worker] = localNoData
		}(w, start, end)
	}

	wgNDVI.Wait()

	// Calcular métricas NDVI
	metricasNDVI.Min = minVals[0]
	metricasNDVI.Max = maxVals[0]
	totalSum := sums[0]

	for i := 1; i < numWorkers; i++ {
		if minVals[i] < metricasNDVI.Min {
			metricasNDVI.Min = minVals[i]
		}
		if maxVals[i] > metricasNDVI.Max {
			metricasNDVI.Max = maxVals[i]
		}
		totalSum += sums[i]
	}

	metricasNDVI.Promedio = totalSum / float64(pixelCount)
	metricasNDVI.Tiempo = time.Since(inicioNDVI)

	// Contabilizar píxeles sin datos
	totalSinDatos := 0
	for _, count := range pixelesSinDatos {
		totalSinDatos += count
	}
	metricasNDVI.PixelesSinDatos = totalSinDatos

	// Procesar color en paralelo
	inicioColor := time.Now()
	ndviColorImg := image.NewRGBA(image.Rect(0, 0, width, height))
	colorPix := ndviColorImg.Pix

	var wgColor sync.WaitGroup
	wgColor.Add(numWorkers)

	for w := range numWorkers {
		start := w * chunkSize
		end := min(start+chunkSize, pixelCount)

		go func(start, end int) {
			defer wgColor.Done()
			for i := start; i < end; i++ {
				rgba := ndviColorOptimized(ndviData[i])
				idx := i * 4
				colorPix[idx] = rgba.R
				colorPix[idx+1] = rgba.G
				colorPix[idx+2] = rgba.B
				colorPix[idx+3] = 255
			}
		}(start, end)
	}

	wgColor.Wait()
	metricasColor.Tiempo = time.Since(inicioColor)

	metricasColor.TamanoImagen = int64(width * height * 4) // 4 bytes por pixel RGBA

	return metricasNDVI, metricasColor, ndviColorImg, nil
}

// PrintMetricsTable imprime una tabla con las métricas de rendimiento
func PrintMetricsTable(metricas []*Metricas) {
	// Análisis de Cuellos de Botella
	fmt.Println("┌ Análisis de Cuellos de Botella ─┬──────────────────┬──────────────────┬──────────────────┬──────────────────┬──────────────────┐")
	fmt.Printf("│ %-12s │ %-16s │ %-16s │ %-16s │ %-16s │ %-16s │ %-16s │\n",
		"Res",
		"TTR NIR",
		"TTR RED",
		"NDVI",
		"Proc. Color",
		"Guardado",
		"Total")
	fmt.Println("├──────────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┤")

	for _, m := range metricas {
		// Formatear la información de resolución con tipo de procesador
		resDisplay := m.Resolucion
		if m.TipoProcesador == "CPU" {
			resDisplay = fmt.Sprintf("%s CPU %dc", m.Resolucion, m.NumHilos)
		} else if m.TipoProcesador == "GPU" {
			resDisplay = fmt.Sprintf("%s GPU", m.Resolucion)
		}

		nirMag, nirUnit := getMagnitudeAndUnit(m.TiempoArchivoNIR + m.TiempoDecodifNIR)
		redMag, redUnit := getMagnitudeAndUnit(m.TiempoArchivoRED + m.TiempoDecodifRED)
		ndviMag, ndviUnit := getMagnitudeAndUnit(m.TiempoNDVI)
		colorMag, colorUnit := getMagnitudeAndUnit(m.TiempoColor)
		guardMag, guardUnit := getMagnitudeAndUnit(m.TiempoGuardado)
		totalMag, totalUnit := getMagnitudeAndUnit(m.TiempoTotal)

		porcNIR := float64(m.TiempoArchivoNIR+m.TiempoDecodifNIR) / float64(m.TiempoTotal) * 100
		porcRED := float64(m.TiempoArchivoRED+m.TiempoDecodifRED) / float64(m.TiempoTotal) * 100
		porcNDVI := float64(m.TiempoNDVI) / float64(m.TiempoTotal) * 100
		porcColor := float64(m.TiempoColor) / float64(m.TiempoTotal) * 100
		porcGuardado := float64(m.TiempoGuardado) / float64(m.TiempoTotal) * 100

		fmt.Printf("│ %-12s │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │\n",
			resDisplay,
			formatNumber(nirMag, 5), nirUnit, formatNumber(porcNIR, 5),
			formatNumber(redMag, 5), redUnit, formatNumber(porcRED, 5),
			formatNumber(ndviMag, 5), ndviUnit, formatNumber(porcNDVI, 5),
			formatNumber(colorMag, 5), colorUnit, formatNumber(porcColor, 5),
			formatNumber(guardMag, 5), guardUnit, formatNumber(porcGuardado, 5),
			formatNumber(totalMag, 5), totalUnit, "100.0") // Total siempre es 100%
	}
	fmt.Println("└──────────────┴─────────┴────────┴─────────┴────────┴─────────┴────────┴─────────┴────────┴─────────┴────────┴─────────┴────────┘")
	fmt.Println()

	// También hay que actualizar la segunda tabla
	fmt.Println("┌ Desglose de Lectura de Imágenes ┬───────────┬───────────┬──────────┬──────────┐")
	fmt.Printf("│ %-12s │ %-16s │ %-9s │ %-9s │ %-8s │ %-8s │\n",
		"Res",
		"Uncovered Region",
		"Tiles NIR",
		"Tiles RED",
		"Total MP",
		"Img Size")
	fmt.Println("├──────────────┼─────────┬────────┼───────────┼───────────┼──────────┼──────────┤")

	for _, m := range metricas {
		// Formatear la información de resolución con tipo de procesador (igual que antes)
		resDisplay := m.Resolucion
		if m.TipoProcesador == "CPU" {
			resDisplay = fmt.Sprintf("%s CPU %dc", m.Resolucion, m.NumHilos)
		} else if m.TipoProcesador == "GPU" {
			resDisplay = fmt.Sprintf("%s GPU", m.Resolucion)
		}

		porcNoData := float64(m.PixelesSinDatos) / float64(m.Pixeles) * 100
		pixelesSinDatosMP := float64(m.PixelesSinDatos) / 1000000

		var totalPixUnidad string
		var totalPixValor float64

		if m.Pixeles >= 1000000 {
			totalPixValor = float64(m.Pixeles) / 1000000
			totalPixUnidad = "MP"
		} else if m.Pixeles >= 1000 {
			totalPixValor = float64(m.Pixeles) / 1000
			totalPixUnidad = "KP"
		} else {
			totalPixValor = float64(m.Pixeles)
			totalPixUnidad = "P"
		}

		tamanoMB := float64(m.TamanoImagen) / (1024 * 1024)

		fmt.Printf("│ %-12s │ %s%-2s │ %s%% │ %-3d tiles │ %-3d tiles │ %s %-2s │ %s MB │\n",
			resDisplay,
			formatNumber(pixelesSinDatosMP, 5), "MP", formatNumber(porcNoData, 5),
			m.NumTilesNIR,
			m.NumTilesRED,
			formatNumber(totalPixValor, 5), totalPixUnidad,
			formatNumber(tamanoMB, 5),
		)
	}
	fmt.Println("└──────────────┴─────────┴────────┴───────────┴───────────┴──────────┴──────────┘")
}

// getMagnitudeAndUnit obtiene la magnitud y unidad adecuada para una duración
func getMagnitudeAndUnit(d time.Duration) (float64, string) {
	if d < time.Microsecond {
		return float64(d.Nanoseconds()), "ns"
	} else if d < time.Millisecond {
		return float64(d.Nanoseconds()) / 1000, "µs"
	} else if d < time.Second {
		return float64(d.Nanoseconds()) / 1000000, "ms"
	} else {
		return d.Seconds(), "s"
	}
}

// formatNumber formatea un número para mostrar en la tabla de métricas
func formatNumber(num float64, desiredLength int) string {
	integerPart := int(math.Floor(math.Abs(num)))
	integerLength := len(strconv.Itoa(integerPart))

	precision := 0
	if integerLength < desiredLength {
		precision = desiredLength - integerLength
		if precision > 0 {
			precision--
		}
	}

	if precision > 0 {
		return fmt.Sprintf("%.*f", precision, num)
	} else {
		return fmt.Sprintf("%d", integerPart)
	}
}

// ProcessNDVI procesa un par de imágenes JP2 (NIR y RED) para generar una imagen NDVI colorizada
// Parámetros:
//   - nirPath: ruta a la imagen NIR (banda B08)
//   - redPath: ruta a la imagen RED (banda B04)
//   - outputPath: ruta para guardar la imagen resultado
//   - useGPU: true para usar GPU, false para CPU
//   - threads: número de hilos a utilizar (solo para CPU)
//
// Devuelve las métricas del procesamiento y cualquier error que ocurra
func ProcessNDVI(nirPath, redPath, outputPath string, useGPU bool, threads int) (*Metricas, error) {
	// 1. Leer imagen NIR
	inicioTotalLectura := time.Now()
	var nirResultado, redResultado *ResultadoBanda
	var err error

	if useGPU {
		fmt.Printf("Leyendo NIR con GPU: %s\n", nirPath)
		nirResultado, err = ReadGPU(nirPath, 1) // El parámetro threads se ignora en GPU
	} else {
		fmt.Printf("Leyendo NIR con CPU (%d hilos): %s\n", threads, nirPath)
		nirResultado, err = ReadCPU(nirPath, threads)
	}

	if err != nil {
		return nil, fmt.Errorf("error leyendo NIR: %v", err)
	}

	// 2. Leer imagen RED
	if useGPU {
		fmt.Printf("Leyendo RED con GPU: %s\n", redPath)
		redResultado, err = ReadGPU(redPath, 1) // El parámetro threads se ignora en GPU
	} else {
		fmt.Printf("Leyendo RED con CPU (%d hilos): %s\n", threads, redPath)
		redResultado, err = ReadCPU(redPath, threads)
	}

	if err != nil {
		return nil, fmt.Errorf("error leyendo RED: %v", err)
	}

	tiempoLectura := time.Since(inicioTotalLectura)

	// 3. Calcular NDVI y colorizar
	fmt.Printf("Calculando NDVI con %d hilos\n", threads)
	metricasNDVI, metricasColor, ndviColorImg, err := CalculateNDVI(nirResultado, redResultado, threads)
	if err != nil {
		// Liberar recursos antes de retornar error
		nirResultado.Free()
		redResultado.Free()
		return nil, fmt.Errorf("error calculando NDVI: %v", err)
	}

	// 4. Guardar imagen resultado
	// Extraer nombre de archivo base desde outputPath
	outputBase := filepath.Base(outputPath)
	outputBase = strings.TrimSuffix(outputBase, filepath.Ext(outputBase))

	fmt.Printf("Guardando imagen resultado en: %s\n", outputPath)
	var tiempoGuardado time.Duration

	// Usar StoreGPU si estamos en modo GPU, StoreCPU en caso contrario
	if useGPU {
		fmt.Printf("Usando codificador GPU para guardar imagen\n")
		tiempoGuardado, err = StoreGPU(ndviColorImg, outputBase)
	} else {
		fmt.Printf("Usando codificador CPU para guardar imagen\n")
		tiempoGuardado, err = StoreCPU(ndviColorImg, outputBase, threads)
	}

	if err != nil {
		return nil, fmt.Errorf("error guardando imagen: %v", err)
	}

	// 5. Registrar y devolver métricas
	var tipoProcesador string
	if useGPU {
		tipoProcesador = "GPU"
	} else {
		tipoProcesador = "CPU"
	}

	defer func() {
		ndviColorImg.Pix = nil // Release pixel buffer
		runtime.GC()
		debug.FreeOSMemory()
	}()

	metrica := &Metricas{
		Resolucion:       outputBase,
		TipoProcesador:   tipoProcesador,
		NumHilos:         threads,
		TiempoLectura:    tiempoLectura,
		TiempoArchivoNIR: nirResultado.Metricas.TiempoArchivo,
		TiempoDecodifNIR: nirResultado.Metricas.TiempoDecodif,
		TiempoArchivoRED: redResultado.Metricas.TiempoArchivo,
		TiempoDecodifRED: redResultado.Metricas.TiempoDecodif,
		NumTilesNIR:      nirResultado.Metricas.NumTiles,
		NumTilesRED:      redResultado.Metricas.NumTiles,
		TiempoNDVI:       metricasNDVI.Tiempo,
		Pixeles:          metricasNDVI.PixelesTotales,
		PixelesSinDatos:  metricasNDVI.PixelesSinDatos,
		NDVIMin:          metricasNDVI.Min,
		NDVIMax:          metricasNDVI.Max,
		NDVIPromedio:     metricasNDVI.Promedio,
		TiempoColor:      metricasColor.Tiempo,
		TamanoImagen:     metricasColor.TamanoImagen,
		TiempoGuardado:   tiempoGuardado,
	}

	// Calcular tiempo total
	metrica.TiempoTotal = tiempoLectura + metricasNDVI.Tiempo + metricasColor.Tiempo + tiempoGuardado

	// Liberar recursos después de usarlos
	nirResultado.Free()
	redResultado.Free()
	return metrica, nil
}

// AgregarMetricas suma los valores de dos métricas
// Se utiliza para acumular métricas en ejecuciones múltiples
func AgregarMetricas(acumulado, nuevo *Metricas) {
	acumulado.TiempoTotal += nuevo.TiempoTotal
	acumulado.TiempoLectura += nuevo.TiempoLectura
	acumulado.TiempoNDVI += nuevo.TiempoNDVI
	acumulado.TiempoColor += nuevo.TiempoColor
	acumulado.TiempoGuardado += nuevo.TiempoGuardado
	acumulado.TiempoArchivoNIR += nuevo.TiempoArchivoNIR
	acumulado.TiempoDecodifNIR += nuevo.TiempoDecodifNIR
	acumulado.TiempoArchivoRED += nuevo.TiempoArchivoRED
	acumulado.TiempoDecodifRED += nuevo.TiempoDecodifRED
	acumulado.PixelesSinDatos += nuevo.PixelesSinDatos
	acumulado.NDVIMin = math.Min(acumulado.NDVIMin, nuevo.NDVIMin)
	acumulado.NDVIMax = math.Max(acumulado.NDVIMax, nuevo.NDVIMax)
	acumulado.NDVIPromedio += nuevo.NDVIPromedio
}

// PromediarMetricas calcula el promedio de las métricas acumuladas
func PromediarMetricas(acumulado *Metricas, numEjecuciones int) *Metricas {
	resultado := *acumulado // Copia todos los valores

	// Dividimos los tiempos entre el número de ejecuciones
	resultado.TiempoTotal /= time.Duration(numEjecuciones)
	resultado.TiempoLectura /= time.Duration(numEjecuciones)
	resultado.TiempoNDVI /= time.Duration(numEjecuciones)
	resultado.TiempoColor /= time.Duration(numEjecuciones)
	resultado.TiempoGuardado /= time.Duration(numEjecuciones)
	resultado.TiempoArchivoNIR /= time.Duration(numEjecuciones)
	resultado.TiempoDecodifNIR /= time.Duration(numEjecuciones)
	resultado.TiempoArchivoRED /= time.Duration(numEjecuciones)
	resultado.TiempoDecodifRED /= time.Duration(numEjecuciones)

	// Promediamos valores numéricos
	resultado.PixelesSinDatos /= numEjecuciones
	resultado.NDVIPromedio /= float64(numEjecuciones)

	// Min/Max y otros valores estáticos se mantienen igual

	return &resultado
}

// CopiarMetricas crea una copia de las métricas
func CopiarMetricas(m *Metricas) *Metricas {
	copia := *m // Copia todos los campos
	return &copia
}

// InicializarMetricasAcumuladas crea una estructura inicial para acumular métricas
func InicializarMetricasAcumuladas(original *Metricas) *Metricas {
	acumulado := CopiarMetricas(original)
	acumulado.NDVIMin = math.MaxFloat64  // Para permitir encontrar el mínimo real
	acumulado.NDVIMax = -math.MaxFloat64 // Para permitir encontrar el máximo real
	return acumulado
}

func main() {
	// Parámetro para número de repeticiones
	numRepeticiones := flag.Int("reps", 10, "Número de repeticiones del benchmark")
	flag.Parse()

	if *numRepeticiones < 1 {
		fmt.Println("El número de repeticiones debe ser al menos 1")
		*numRepeticiones = 1
	}

	fmt.Printf("Ejecutando benchmark con %d repeticiones\n", *numRepeticiones)

	// Definir configuraciones a procesar
	configuraciones := []struct {
		NIR        string
		RED        string
		Resolucion string
	}{
		{"../input_images/COMPLETE_B08_10m.jp2", "../input_images/COMPLETE_B04_10m.jp2", "10m"},
		{"../input_images/COMPLETE_B08_20m.jp2", "../input_images/COMPLETE_B04_20m.jp2", "20m"},
		{"../input_images/COMPLETE_B08_60m.jp2", "../input_images/COMPLETE_B04_60m.jp2", "60m"},
	}

	// Definir los diferentes números de núcleos a probar
	nucleosAProbar := []int{1, 2, 4, 8, 12, 16}

	// Obtener número máximo de CPUs disponibles
	maxCPUs := runtime.NumCPU()
	fmt.Printf("CPUs disponibles: %d\n\n", maxCPUs)

	// Almacenar métricas para cada resolución y configuración de núcleos
	metricasPromedio := make([]*Metricas, 0, len(configuraciones)*(len(nucleosAProbar)+1)) // +1 para GPU

	for _, cfg := range configuraciones {

		fmt.Printf("=== Procesando resolución %s ===\n", cfg.Resolucion)

		// Procesar con diferentes configuraciones de CPU
		for _, numNucleos := range nucleosAProbar {
			// Saltarse configuraciones que excedan el número de CPUs disponibles
			if numNucleos > maxCPUs {
				fmt.Printf("\nSaltando prueba con %d núcleos (máximo disponible: %d)\n",
					numNucleos, maxCPUs)
				continue
			}

			fmt.Printf("\n>> Modo CPU (%d núcleos)\n", numNucleos)

			var metricasAcumuladas *Metricas
			var errAcumulado error

			// Ejecutar el benchmark múltiples veces
			for i := 1; i <= *numRepeticiones; i++ {
				fmt.Printf("  Ejecución %d/%d... ", i, *numRepeticiones)
				printMemUsage()
				metricaCPU, err := ProcessNDVI(
					cfg.NIR,
					cfg.RED,
					cfg.Resolucion,
					false, // useGPU = false
					numNucleos,
				)

				if err != nil {
					fmt.Printf("Error: %v\n", err)
					errAcumulado = err
					continue
				}

				fmt.Printf("Completado en %v\n", metricaCPU.TiempoTotal)

				if metricasAcumuladas == nil {
					metricasAcumuladas = InicializarMetricasAcumuladas(metricaCPU)
				} else {
					AgregarMetricas(metricasAcumuladas, metricaCPU)
				}

				// Forzar recolección de basura para liberar memoria
				runtime.GC()
			}

			if metricasAcumuladas != nil {
				metricaPromedio := PromediarMetricas(metricasAcumuladas, *numRepeticiones)
				metricasPromedio = append(metricasPromedio, metricaPromedio)
				fmt.Printf("  Tiempo promedio: %v\n", metricaPromedio.TiempoTotal)
			} else if errAcumulado != nil {
				fmt.Printf("Error procesando %s con %d CPUs: %v\n",
					cfg.Resolucion, numNucleos, errAcumulado)
			}
		}

		// Procesar con GPU si está disponible
		fmt.Printf("\n>> Modo GPU\n")

		var metricasAcumuladasGPU *Metricas
		var errAcumuladoGPU error

		// Ejecutar el benchmark múltiples veces para GPU
		for i := 1; i <= *numRepeticiones; i++ {
			fmt.Printf("  Ejecución %d/%d... ", i, *numRepeticiones)
			printMemUsage()
			metricaGPU, err := ProcessNDVI(
				cfg.NIR,
				cfg.RED,
				cfg.Resolucion,
				true, // useGPU = true
				1,    // threads no se usa en GPU
			)

			if err != nil {
				fmt.Printf("Error: %v\n", err)
				errAcumuladoGPU = err
				continue
			}

			fmt.Printf("Completado en %v\n", metricaGPU.TiempoTotal)

			if metricasAcumuladasGPU == nil {
				metricasAcumuladasGPU = InicializarMetricasAcumuladas(metricaGPU)
			} else {
				AgregarMetricas(metricasAcumuladasGPU, metricaGPU)
			}
		}

		if metricasAcumuladasGPU != nil {
			metricaPromedioGPU := PromediarMetricas(metricasAcumuladasGPU, *numRepeticiones)
			metricasPromedio = append(metricasPromedio, metricaPromedioGPU)
			fmt.Printf("  Tiempo promedio: %v\n", metricaPromedioGPU.TiempoTotal)
		} else if errAcumuladoGPU != nil {
			fmt.Printf("Error procesando %s con GPU: %v\n", cfg.Resolucion, errAcumuladoGPU)
			fmt.Printf("¿Está disponible la GPU con soporte CUDA y nvJPEG2000?\n")
		}

		fmt.Println("\n-----------------------------------")
	}

	// Imprimir tabla de métricas
	if len(metricasPromedio) > 0 {
		fmt.Printf("\n--- RESULTADOS DEL BENCHMARK (Promedio de %d ejecuciones) ---\n", *numRepeticiones)
		PrintMetricsTable(metricasPromedio)

		// También generar un resumen de escalabilidad para cada resolución
		fmt.Println("\n--- ANÁLISIS DE ESCALABILIDAD ---")
		for _, cfg := range configuraciones {
			metricasPorResolucion := make(map[string][]*Metricas)
			for _, m := range metricasPromedio {
				// Extraer la resolución base (sin sufijo _cpu_XXc o _gpu)
				baseRes := strings.SplitN(m.Resolucion, "_", 2)[0]
				if baseRes == cfg.Resolucion {
					metricasPorResolucion[baseRes] = append(metricasPorResolucion[baseRes], m)
				}
			}

			for res, ms := range metricasPorResolucion {
				if len(ms) > 0 {
					fmt.Printf("\nResolución: %s\n", res)
					fmt.Println("┌────────────┬────────────┬─────────┬────────────┐")
					fmt.Printf("│ %-10s │ %-10s │ %-7s │ %-10s │\n",
						"Procesador", "Tiempo (s)", "Speedup", "Eficiencia")
					fmt.Println("├────────────┼────────────┼─────────┼────────────┤")

					// Encontrar la métrica de referencia (1 núcleo)
					var tiempoBase float64
					for _, m := range ms {
						if m.TipoProcesador == "CPU" && m.NumHilos == 1 {
							tiempoBase = m.TiempoTotal.Seconds()
							break
						}
					}

					// Si no encontramos métrica de 1 núcleo, usamos la primera disponible
					if tiempoBase == 0 && len(ms) > 0 {
						tiempoBase = ms[0].TiempoTotal.Seconds()
					}

					// Mostrar métricas ordenadas por procesador
					for _, m := range ms {
						tiempo := m.TiempoTotal.Seconds()
						speedup := tiempoBase / tiempo
						eficiencia := 0.0

						if m.TipoProcesador == "CPU" {
							eficiencia = speedup / float64(m.NumHilos)
						} else {
							eficiencia = speedup // Para GPU no calculamos eficiencia
						}

						procLabel := fmt.Sprintf("%s %d", m.TipoProcesador, m.NumHilos)
						if m.TipoProcesador == "GPU" {
							procLabel = "GPU"
						}

						fmt.Printf("│ %-10s │ %10.3f │ %7.2f │ %10.2f │\n",
							procLabel, tiempo, speedup, eficiencia)
					}
					fmt.Println("└────────────┴────────────┴─────────┴────────────┘")
				}
			}
		}
	} else {
		fmt.Println("\nNo se pudo completar ninguna operación con éxito.")
	}
}
