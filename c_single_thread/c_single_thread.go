package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

// #cgo CFLAGS: -I/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/include
// #cgo LDFLAGS: -L/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/lib -lopenjp2
// #include <openjpeg-2.5/openjpeg.h>
// #include <stdlib.h>
// #include <string.h>
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

type Metricas struct {
	Resolucion       string
	TiempoTotal      time.Duration
	TiempoLectura    time.Duration
	TiempoNDVI       time.Duration
	TiempoColor      time.Duration
	TiempoGuardado   time.Duration
	TiempoArchivo    time.Duration
	TiempoDecodif    time.Duration
	TiempoArchivoNIR time.Duration // Nuevo campo
	TiempoArchivoRED time.Duration // Nuevo campo
	TiempoDecodifNIR time.Duration // Nuevo campo
	TiempoDecodifRED time.Duration // Nuevo campo
	Pixeles          int
	PixelesSinDatos  int // Nuevo campo
	TamanoImagen     int64
	NDVIMin          float64
	NDVIMax          float64
	NDVIPromedio     float64
	NumTilesNIR      int // Nuevo campo
	NumTilesRED      int // Nuevo campo
}

// MetricasLectura contiene las métricas asociadas a la lectura de un archivo JP2
type MetricasLectura struct {
	TiempoArchivo time.Duration // Tiempo de apertura del archivo
	TiempoDecodif time.Duration // Tiempo de decodificación
	NumTiles      int           // Número de tiles en la imagen
	TiempoTotal   time.Duration // Tiempo total de lectura
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

// MetricasGuardado contiene las métricas del proceso de guardado
type MetricasGuardado struct {
	Tiempo time.Duration
}

// MetricasProcesoCompleto agrupa todas las métricas del proceso
type MetricasProcesoCompleto struct {
	Resolucion  string
	NIR         MetricasLectura
	RED         MetricasLectura
	NDVI        MetricasNDVI
	Color       MetricasColor
	Guardado    MetricasGuardado
	TiempoTotal time.Duration
}

// JP2Image representa una imagen JPEG2000 decodificada
type JP2Image struct {
	Width, Height int
	Components    int
	Data          [][]float32 // Un slice por componente
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

// readJP2Direct: Simplificar la firma devolviendo una estructura ResultadoBanda
func readJP2Direct(filePath string, threads int) (*ResultadoBanda, error) {
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

		// Copiamos los datos normalizados
		factor := math.Pow(2, float64(comp.prec)-1)
		for j := range jp2Image.Width * jp2Image.Height {
			// Obtenemos el valor del componente
			compData := (*[1 << 30]C.int)(unsafe.Pointer(comp.data))
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

// leerImagenes: Simplificar la firma devolviendo las imágenes y sus métricas agrupadas
func leerImagenesCPU(archivoNIR, archivoRED string, numCPUs int) (*ResultadoBanda, *ResultadoBanda, time.Duration, error) {
	inicioLectura := time.Now()

	// Leer NIR
	nirResultado, err := readJP2Direct(archivoNIR, numCPUs)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("error al leer NIR: %v", err)
	}

	// Leer RED
	redResultado, err := readJP2Direct(archivoRED, numCPUs)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("error al leer RED: %v", err)
	}

	tiempoTotal := time.Since(inicioLectura)

	return nirResultado, redResultado, tiempoTotal, nil
}

// guardarImagenes guarda las imágenes de NDVI en formato PNG
func guardarImagenes(ndviColorImg *image.RGBA, resolucion string) (time.Duration, error) {
	inicioGuardado := time.Now()

	// Crear directorio si no existe
	os.MkdirAll("./go_jp2_direct", 0755)

	// Nombre del archivo de salida (solo color)
	nombreColor := fmt.Sprintf("./go_jp2_direct/ndvi_%s_color.png", resolucion)

	// Guardar solo la imagen color
	colorFile, err := os.Create(nombreColor)
	if err != nil {
		return 0, err
	}
	defer colorFile.Close()

	// Usar compresión rápida para mejor rendimiento
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	err = encoder.Encode(colorFile, ndviColorImg)

	tiempoGuardado := time.Since(inicioGuardado)
	return tiempoGuardado, err
}

// procesarNDVIMulti: Simplificar para devolver métricas NDVI y color agrupadas
func procesarNDVIMulti(nirResultado, redResultado *ResultadoBanda, resolucion string, numCPUs int) (*MetricasNDVI, *MetricasColor, *image.RGBA, error) {
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

	// Después de wgNDVI.Wait(), sumar los conteos de píxeles sin datos:
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

func imprimirTablaRendimiento(metricas []*Metricas) {
	// Análisis de Cuellos de Botella
	fmt.Println("┌ Análisis de Cuellos de Botella ───────────────┬──────────────────┬──────────────────┬──────────────────┬──────────────────┐")
	fmt.Printf("│ %-7s │ %-16s │ %-16s │ %-16s │ %-16s │ %-16s │ %-16s │\n",
		"Res",
		"TTR NIR",
		"TTR RED",
		"NDVI",
		"Proc. Color",
		"Guardado",
		"Total")
	fmt.Println("├─────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┤")

	for _, m := range metricas {
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

		fmt.Printf("│ %-7s │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │\n",
			m.Resolucion,
			formatNumber(nirMag), nirUnit, formatNumber(porcNIR),
			formatNumber(redMag), redUnit, formatNumber(porcRED),
			formatNumber(ndviMag), ndviUnit, formatNumber(porcNDVI),
			formatNumber(colorMag), colorUnit, formatNumber(porcColor),
			formatNumber(guardMag), guardUnit, formatNumber(porcGuardado),
			formatNumber(totalMag), totalUnit, "100.0") // Total siempre es 100%
	}
	fmt.Println("└─────────┴─────────┴────────┴───────────┴───────────┴──────────┴──────────┘")
	fmt.Println()

	// Desglose de Lectura de Imágenes
	fmt.Println("┌ Desglose de Lectura de Imágenes ───────┬───────────┬──────────┬──────────┐")
	fmt.Printf("│ %-7s │ %-16s │ %-9s │ %-9s │ %-8s │ %-8s │\n",
		"Res",
		"Uncovered Region",
		"Tiles NIR",
		"Tiles RED",
		"Total MP",
		"Img Size")
	fmt.Println("├─────────┼─────────┬────────┼───────────┼───────────┼──────────┼──────────┤")

	for _, m := range metricas {
		porcNoData := float64(m.PixelesSinDatos) / float64(m.Pixeles) * 100

		// Calcula los megapíxeles sin datos
		pixelesSinDatosMP := float64(m.PixelesSinDatos) / 1000000

		// Formatear el total de píxeles
		var totalPixUnidad string
		var totalPixValor float64

		if m.Pixeles >= 1000000 {
			totalPixValor = float64(m.Pixeles) / 1000000
			totalPixUnidad = "MP" // Megapíxeles
		} else if m.Pixeles >= 1000 {
			totalPixValor = float64(m.Pixeles) / 1000
			totalPixUnidad = "KP" // Kilopíxeles
		} else {
			totalPixValor = float64(m.Pixeles)
			totalPixUnidad = "P" // Píxeles
		}

		tamanoMB := float64(m.TamanoImagen) / (1024 * 1024)

		fmt.Printf("│ %-7s │ %s%-2s │ %s%% │ %-3d tiles │ %-3d tiles │ %s %-2s │ %s MB │\n",
			m.Resolucion,
			formatNumber(pixelesSinDatosMP), "MP", formatNumber(porcNoData), // Ahora muestra MP de región sin cobertura
			m.NumTilesNIR,
			m.NumTilesRED,
			formatNumber(totalPixValor), totalPixUnidad,
			formatNumber(tamanoMB),
		)
	}
	fmt.Println("└─────────┴─────────┴────────┴───────────┴───────────┴──────────┴──────────┘")
}

// Función auxiliar para obtener la magnitud y la unidad de una duración
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

func formatNumber(num float64) string {
	integerPart := int(math.Floor(math.Abs(num)))
	integerLength := len(strconv.Itoa(integerPart))

	var precision int
	switch integerLength {
	case 3:
		precision = 1
	case 2:
		precision = 2
	case 1:
		precision = 3
	default:
		// Manejar otros casos si es necesario
		return fmt.Sprintf("%g", num) // Formato general
	}
	return fmt.Sprintf("%.*f", precision, num)
}

func main() {
	// Definir configuraciones a procesar
	configuraciones := []struct {
		NIR        string
		RED        string
		Resolucion string
	}{
		{"../input_images/B08_10m.jp2", "../input_images/B04_10m.jp2", "10m"},
		{"../input_images/B08_20m.jp2", "../input_images/B04_20m.jp2", "20m"},
		{"../input_images/B08_60m.jp2", "../input_images/B04_60m.jp2", "60m"},
		{"../input_images_2/B08_10m.jp2", "../input_images_2/B04_10m.jp2", "10m"},
		{"../input_images_2/B08_20m.jp2", "../input_images_2/B04_20m.jp2", "20m"},
		{"../input_images_2/B08_60m.jp2", "../input_images_2/B04_60m.jp2", "60m"},
	}

	// Get CPU count
	numCPUs := runtime.NumCPU()
	fmt.Printf("CPUs disponibles: %d\n\n", numCPUs)

	// Configuraciones de cores para probar
	coresConfigs := []int{12}

	// Almacenar métricas para cada resolución y modo
	metricas := make([]*Metricas, 0, len(configuraciones)*(len(coresConfigs)+1))

	// Multi-threaded con diferentes configuraciones de cores
	for _, cores := range coresConfigs {
		fmt.Printf("\nEjecutando benchmarks en modo multi-thread con %d cores...\n", cores)
		for _, cfg := range configuraciones {
			// 1. Leer imágenes
			nirResultado, redResultado, tiempoLectura, err := leerImagenesCPU(cfg.NIR, cfg.RED, cores)

			if err != nil {
				fmt.Printf("Error leyendo imágenes %s (multi-%d): %v\n", cfg.Resolucion, cores, err)
				continue
			}

			// 2. Procesar NDVI
			metricasNDVI, metricasColor, ndviColorImg, err := procesarNDVIMulti(nirResultado, redResultado, cfg.Resolucion, cores)
			if err != nil {
				fmt.Printf("Error procesando %s (multi-%d): %v\n", cfg.Resolucion, cores, err)
				continue
			}

			// Crear estructura de métricas
			metrica := &Metricas{
				Resolucion:       cfg.Resolucion,
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
			}

			// 3. Guardar imágenes
			tiempoGuardado, err := guardarImagenes(ndviColorImg, cfg.Resolucion)
			if err != nil {
				fmt.Printf("Error guardando imágenes %s (multi-%d): %v\n", cfg.Resolucion, cores, err)
				continue
			}
			metrica.TiempoGuardado = tiempoGuardado

			// Actualizar el tiempo total incluyendo todas las fases
			metrica.TiempoTotal = metrica.TiempoLectura + metrica.TiempoNDVI + metrica.TiempoColor + metrica.TiempoGuardado

			metrica.Resolucion = fmt.Sprintf("%s-M%d", cfg.Resolucion, cores) // Indicador de modo multi con # de cores
			metricas = append(metricas, metrica)
		}
	}

	// Imprimir tabla de rendimiento
	fmt.Println("\n--- RESULTADOS DEL BENCHMARK ---")
	fmt.Println("S: Single-thread, M#: Multi-thread (# cores)")
	imprimirTablaRendimiento(metricas)
}
