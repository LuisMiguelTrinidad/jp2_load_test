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