# JP2 Processing Benchmark

Este proyecto implementa un benchmark para el procesamiento de imágenes JPEG2000 (JP2) y el cálculo de índice NDVI (Índice de Vegetación de Diferencia Normalizada) utilizando tanto CPU como GPU.

## Estructura del proyecto

```
jp2_processing/
│
├── cmd/
│   └── benchmark/
│       └── main.go           # Punto de entrada para el benchmark
│
├── pkg/
│   ├── jp2/
│   │   ├── reader.go         # Interfaz genérica para lectura
│   │   ├── cpu/
│   │   │   └── reader.go     # Implementación de lectura con OpenJPEG
│   │   ├── gpu/
│   │   │   └── reader.go     # Implementación de lectura con nvJPEG2K
│   │   ├── writer.go         # Interfaz para escritura
│   │   ├── cpu/
│   │   │   └── writer.go     # Implementación de escritura con OpenJPEG
│   │   └── gpu/
│   │       └── writer.go     # Implementación de escritura con nvJPEG2K
│   │
│   ├── ndvi/
│   │   ├── calculator.go     # Cálculo de NDVI
│   │   └── colorizer.go      # Colorización de valores NDVI
│   │
│   ├── metrics/
│   │   ├── types.go          # Estructuras de métricas
│   │   ├── collector.go      # Recolección de métricas
│   │   └── reporter.go       # Generación de informes
│   │
│   └── utils/
│       └── memory.go         # Utilidades para manejo de memoria
│
├── internal/
│   └── cgo/
│       ├── openjpeg/         # Bindings CGO para OpenJPEG
│       └── nvjpeg2k/         # Bindings CGO para nvJPEG2K
│
├── config/
│   └── gradient.go           # Configuración del gradiente NDVI
│
└── test/
    └── testdata/            # Imágenes pequeñas para pruebas
```

## Requisitos

- Go 1.16 o superior
- OpenJPEG 2.5.x (para procesamiento CPU)
- NVIDIA CUDA y nvJPEG2K (para procesamiento GPU)

## Compilación

```
go build -o jp2_ndvi_benchmark ./cmd/benchmark
```

## Uso

```
./jp2_ndvi_benchmark -nir=ruta/a/banda_nir.jp2 -red=ruta/a/banda_red.jp2 -res=etiqueta_resolución -threads=4 -gpu -iter=3
```

### Parámetros

- `-nir`: Ruta al archivo JP2 para la banda NIR (infrarrojo cercano)
- `-red`: Ruta al archivo JP2 para la banda RED (rojo)
- `-res`: Etiqueta de resolución para los informes (ej. "10m", "20m", "60m")
- `-threads`: Número de hilos para procesamiento CPU (por defecto: número de núcleos disponibles)
- `-cpu`: Usar CPU para el procesamiento (por defecto: true)
- `-gpu`: Usar GPU para el procesamiento (por defecto: false)
- `-iter`: Número de iteraciones para el benchmark (por defecto: 1)

## Resultados

El benchmark genera un informe detallado que incluye:

1. Análisis de cuellos de botella: Tiempo y porcentaje para cada etapa del procesamiento.
2. Desglose de lectura de imágenes: Información sobre las regiones sin cobertura, tiles y tamaño.
3. Análisis de escalabilidad: Comparación de rendimiento entre diferentes configuraciones.

## Características

- Procesamiento paralelo para cálculo de NDVI
- Soporte para aceleración GPU mediante nvJPEG2K
- Medición detallada del rendimiento
- Colorización de NDVI según esquema de color estándar

## Benchmarks

The following benchmark results were obtained processing a 5490x5490 pixel image:

### Bottleneck Analysis

| TTR NIR | % | TTR RED | % | NDVI | % | Color Proc. | % | Saving | % | Total | % |
|---------|---|---------|---|------|---|-------------|---|--------|---|-------|---|
| 1.506s  | 23.33% | 1.544s  | 23.93% | 129.8ms | 2.011% | 240.8ms | 3.731% | 2.999s  | 46.48% | 6.453s  | 100.0% |
| 1.027s  | 24.57% | 1.048s  | 25.07% | 116.1ms | 2.777% | 146.8ms | 3.510% | 1.815s  | 43.42% | 4.181s  | 100.0% |
| 771.7ms | 25.47% | 773.5ms | 25.53% | 118.5ms | 3.910% | 116.5ms | 3.847% | 1.224s  | 40.39% | 3.029s  | 100.0% |
| 769.7ms | 27.85% | 776.8ms | 28.11% | 117.2ms | 4.241% | 96.13ms | 3.479% | 978.9ms | 35.42% | 2.763s  | 100.0% |
| 751.5ms | 28.78% | 765.4ms | 29.31% | 117.5ms | 4.498% | 84.22ms | 3.225% | 874.8ms | 33.50% | 2.611s  | 100.0% |
| 741.1ms | 28.23% | 747.9ms | 28.49% | 119.7ms | 4.560% | 85.27ms | 3.249% | 903.3ms | 34.41% | 2.625s  | 100.0% |
| 253.6ms | 21.61% | 197.6ms | 16.85% | 136.8ms | 11.66% | 241.7ms | 20.60% | 291.0ms | 24.80% | 1.173s  | 100.0% |

### Image Reading Breakdown

| Uncovered Region | % | NIR Tiles | RED Tiles | Total MP | Img Size |
|------------------|---|-----------|-----------|----------|----------|
| 20.43MP | 67.78% | 81 tiles | 81 tiles | 30.14 MP | 115.0 MB |

### Scalability Analysis

| Processor | Time (s) | Speedup | Efficiency |
|-----------|----------|---------|------------|
| CPU 1     | 6.453    | 1.00    | 1.00       |
| CPU 2     | 4.181    | 1.54    | 0.77       |
| CPU 4     | 3.029    | 2.13    | 0.53       |
| CPU 8     | 2.763    | 2.34    | 0.29       |
| CPU 12    | 2.611    | 2.47    | 0.21       |
| CPU 16    | 2.625    | 2.46    | 0.15       |
| GPU       | 1.173    | 5.50    | 5.50       |