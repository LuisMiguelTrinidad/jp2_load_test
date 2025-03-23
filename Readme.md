# Resultados de Benchmark de Procesamiento NDVI con JPEG2000

Este informe presenta los resultados del benchmark del procesamiento NDVI (Índice de Vegetación de Diferencia Normalizada) con imágenes JPEG2000, comparando el rendimiento entre CPU (con diferentes números de hilos) y GPU.

## Descripción del Proyecto

Este proyecto implementa un procesador eficiente de imágenes JPEG2000 para calcular el índice NDVI utilizando dos bandas satelitales: infrarrojo cercano (NIR, banda B08) y rojo (RED, banda B04) de imágenes Sentinel-2. El NDVI se calcula como:

```
NDVI = (NIR - RED) / (NIR + RED)
```

Se proporciona una implementación en Go que permite:
- Decodificación paralela de imágenes JPEG2000 con OpenJPEG (CPU)
- Decodificación acelerada con nvJPEG2K de NVIDIA (GPU)
- Procesamiento paralelo para cálculo de NDVI
- Colorización según una escala de gradiente predefinida

## Estructura del Proyecto

```
/home/luismi/Escritorio/jp2_load_test/
├── c_single_thread.go       # Implementación principal Go
├── go_jp2_direct/           # Directorio para imágenes de salida (creado automáticamente)
├── input_images/            # Imágenes Sentinel-2 de ejemplo
│   ├── COMPLETE_B04_10m.jp2 # Banda roja (10m)
│   ├── COMPLETE_B08_10m.jp2 # Banda NIR (10m)
│   ├── COMPLETE_B04_20m.jp2 # Banda roja (20m)
│   ├── COMPLETE_B08_20m.jp2 # Banda NIR (20m)
│   ├── COMPLETE_B04_60m.jp2 # Banda roja (60m)
│   └── COMPLETE_B08_60m.jp2 # Banda NIR (60m)
└── Readme.md                # Documentación y resultados
```

## Requisitos del Sistema

### Dependencias para CPU:
- Go 1.18 o superior
- OpenJPEG 2.5.0 o superior
- GCC/compilador C

### Dependencias adicionales para GPU:
- CUDA Toolkit 11.0 o superior
- nvJPEG2K (parte del SDK de NVIDIA)
- GPU compatible con CUDA

## Instalación

### 1. Instalar Go
```bash
# Ubuntu/Debian
sudo apt-get install golang

# Arch Linux
sudo pacman -S go

# CentOS/RHEL
sudo yum install golang

# macOS con Homebrew
brew install go
```

### 2. Instalar OpenJPEG
```bash
# Ubuntu/Debian
sudo apt-get install libopenjp2-7-dev

# Arch Linux
sudo pacman -S openjpeg2

# CentOS/RHEL
sudo yum install openjpeg2-devel

# macOS con Homebrew
brew install openjpeg
```

### 3. Instalar CUDA y nvJPEG2K (para soporte GPU)

1. Instalar CUDA Toolkit:
   - Descargue desde https://developer.nvidia.com/cuda-downloads
   - Siga las instrucciones específicas para su sistema

2. Instalar nvJPEG2K:
   - nvJPEG2K se incluye en NVIDIA SDK
   - Asegúrese de que el archivo `libnvjpeg2k.so` está en la ruta de búsqueda de bibliotecas

   ```bash
   # Verificar la instalación de CUDA
   nvcc --version

   # Verificar si la biblioteca nvJPEG2K está disponible
   find /usr -name "*nvjpeg2k*"

   # Si no se encuentra, puede necesitar instalarla desde el paquete CUDA extras
   sudo apt-get install cuda-nvjpeg2k

   # Añadir ruta a las bibliotecas de CUDA
   echo 'export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/local/cuda/lib64' >> ~/.bashrc
   source ~/.bashrc
   ```

### 4. Clonar y compilar el proyecto
```bash
git clone https://github.com/usuario/jp2_load_test.git
cd jp2_load_test
go build c_single_thread.go
```

## Uso

### Procesamiento básico
```bash
./c_single_thread -nir input_images/COMPLETE_B08_10m.jp2 -red input_images/COMPLETE_B04_10m.jp2 -threads 8 -gpu=false
```

### Benchmark completo
```bash
./c_single_thread -reps 10 -all
```

### Opciones disponibles
- `-nir`: Ruta al archivo JP2 de la banda infrarroja
- `-red`: Ruta al archivo JP2 de la banda roja
- `-out`: Ruta para guardar la imagen resultante
- `-threads`: Número de hilos a utilizar (CPU)
- `-gpu`: Usar GPU (`true`) o CPU (`false`)
- `-reps`: Número de repeticiones para el benchmark
- `-all`: Ejecutar todos los benchmarks (CPU y GPU)

## Implementaciones Técnicas

### Implementación CPU (OpenJPEG)

Utiliza la biblioteca OpenJPEG para decodificar imágenes JPEG2000 con las siguientes características:
- Procesamiento multihilo configurado por el parámetro `-threads`
- Decodificación paralela de las bandas NIR y RED
- Manejo óptimo de los tiles (unidades de compresión internas de JPEG2000)
- Normalización de valores según la profundidad de bits de la imagen

### Implementación GPU (nvJPEG2K)

Utiliza la biblioteca nvJPEG2K de NVIDIA para:
- Decodificación acelerada por hardware de imágenes JPEG2000
- Transferencia optimizada memoria CPU-GPU
- Procesamiento paralelo por tile en la GPU
- Soporte para diferentes formatos de precisión (8-bit, 16-bit)

## Resultados del Benchmark

### Análisis de Cuellos de Botella

| Resolución   | TTR NIR          | TTR RED          | NDVI             | Proc. Color      | Guardado         | Total            |
|--------------|------------------|------------------|------------------|------------------|------------------|------------------|
| 10m CPU 1c   | 12.78s  (46.91%) | 12.22s  (44.85%) | 218.4ms (0.802%) | 975.2ms (3.580%) | 1.050s  (3.856%) | 27.24s  (100.0%) |
| 10m CPU 2c   | 6.844s  (45.29%) | 6.547s  (43.32%) | 102.7ms (0.679%) | 566.7ms (3.750%) | 1.052s  (6.961%) | 15.11s  (100.0%) |
| 10m CPU 4c   | 3.802s  (42.41%) | 3.647s  (40.68%) | 88.25ms (0.984%) | 377.6ms (4.212%) | 1.050s  (11.71%) | 8.965s  (100.0%) |
| 10m CPU 8c   | 2.386s  (38.91%) | 2.297s  (37.45%) | 107.4ms (1.751%) | 288.3ms (4.700%) | 1.054s  (17.18%) | 6.134s  (100.0%) |
| 10m CPU 12c  | 2.101s  (37.50%) | 2.023s  (36.12%) | 114.0ms (2.036%) | 310.6ms (5.545%) | 1.052s  (18.78%) | 5.601s  (100.0%) |
| 10m CPU 16c  | 1.903s  (36.63%) | 1.831s  (35.24%) | 118.6ms (2.282%) | 289.6ms (5.575%) | 1.052s  (20.24%) | 5.196s  (100.0%) |
| 10m GPU      | 2.871s  (28.92%) | 2.827s  (28.48%) | 185.7ms (1.871%) | 986.4ms (9.936%) | 3.045s  (30.68%) | 9.928s  (100.0%) |
| 20m CPU 1c   | 3.652s  (46.30%) | 3.668s  (46.51%) | 46.77ms (0.593%) | 252.4ms (3.200%) | 267.3ms (3.389%) | 7.888s  (100.0%) |
| 20m CPU 2c   | 2.181s  (45.22%) | 2.197s  (45.54%) | 27.78ms (0.576%) | 150.2ms (3.115%) | 266.4ms (5.524%) | 4.823s  (100.0%) |
| 20m CPU 4c   | 1.261s  (43.13%) | 1.258s  (43.05%) | 25.47ms (0.871%) | 109.6ms (3.748%) | 267.8ms (9.161%) | 2.923s  (100.0%) |
| 20m CPU 8c   | 828.6ms (40.54%) | 825.3ms (40.38%) | 26.98ms (1.320%) | 94.99ms (4.648%) | 266.7ms (13.05%) | 2.044s  (100.0%) |
| 20m CPU 12c  | 691.9ms (39.09%) | 695.5ms (39.30%) | 28.54ms (1.612%) | 85.39ms (4.825%) | 267.1ms (15.09%) | 1.770s  (100.0%) |
| 20m CPU 16c  | 625.2ms (38.36%) | 627.9ms (38.53%) | 29.62ms (1.817%) | 77.83ms (4.775%) | 267.5ms (16.41%) | 1.630s  (100.0%) |
| 20m GPU      | 276.8ms (18.21%) | 266.9ms (17.56%) | 46.35ms (3.049%) | 258.4ms (17.00%) | 667.4ms (43.91%) | 1.520s  (100.0%) |
| 60m CPU 1c   | 410.3ms (46.46%) | 410.0ms (46.42%) | 7.192ms (0.814%) | 33.10ms (3.748%) | 22.27ms (2.522%) | 883.2ms (100.0%) |
| 60m CPU 2c   | 433.6ms (47.20%) | 434.6ms (47.30%) | 3.691ms (0.402%) | 22.89ms (2.492%) | 23.47ms (2.555%) | 918.8ms (100.0%) |
| 60m CPU 4c   | 225.3ms (45.12%) | 228.9ms (45.84%) | 3.389ms (0.679%) | 18.86ms (3.776%) | 22.36ms (4.478%) | 499.4ms (100.0%) |
| 60m CPU 8c   | 147.6ms (43.49%) | 147.1ms (43.34%) | 2.675ms (0.788%) | 13.13ms (3.868%) | 28.18ms (8.306%) | 339.3ms (100.0%) |
| 60m CPU 12c  | 242.8ms (46.37%) | 234.5ms (44.79%) | 2.680ms (0.512%) | 11.47ms (2.191%) | 31.15ms (5.948%) | 523.6ms (100.0%) |
| 60m CPU 16c  | 297.0ms (46.25%) | 296.8ms (46.21%) | 2.814ms (0.438%) | 10.64ms (1.657%) | 33.86ms (5.272%) | 642.3ms (100.0%) |
| 60m GPU      | 64.35ms (26.36%) | 60.45ms (24.76%) | 6.040ms (2.474%) | 32.52ms (13.32%) | 78.80ms (32.27%) | 244.2ms (100.0%) |

### Desglose de Lectura de Imágenes

| Resolución   | Uncovered Region | Tiles NIR    | Tiles RED    | Total MP     | Img Size    |
|--------------|------------------|--------------|--------------|--------------|-------------|
| 10m CPU 1c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 2c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 4c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 8c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 12c  | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 16c  | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m GPU      | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 20m CPU 1c   | 0.000MP (0.000%) | 81  tiles    | 81  tiles    | 30.14 MP     | 115.0 MB    |
| 20m CPU 2c   | 0.000MP (0.000%) | 81  tiles    | 81  tiles    | 30.14 MP     | 115.0 MB    |
| 20m CPU 4c   | 0.000MP (0.000%) | 81  tiles    | 81  tiles    | 30.14 MP     | 115.0 MB    |
| 20m CPU 8c   | 0.000MP (0.000%) | 81  tiles    | 81  tiles    | 30.14 MP     | 115.0 MB    |
| 20m CPU 12c  | 0.000MP (0.000%) | 81  tiles    | 81  tiles    | 30.14 MP     | 115.0 MB    |
| 20m CPU 16c  | 0.000MP (0.000%) | 81  tiles    | 81  tiles    | 30.14 MP     | 115.0 MB    |
| 20m GPU      | 0.000MP (0.000%) | 81  tiles    | 81  tiles    | 30.14 MP     | 115.0 MB    |
| 60m CPU 1c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 2c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 4c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 8c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 12c  | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 16c  | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m GPU      | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |

## Análisis de Escalabilidad

### Resolución: 10m

| Procesador | Tiempo (s) | Speedup | Eficiencia |
|------------|------------|---------|------------|
| CPU 1      | 27.239     | 1.00    | 1.00       |
| CPU 2      | 15.113     | 1.80    | 0.90       |
| CPU 4      | 8.965      | 3.04    | 0.76       |
| CPU 8      | 6.134      | 4.44    | 0.56       |
| CPU 12     | 5.601      | 4.86    | 0.41       |
| CPU 16     | 5.196      | 5.24    | 0.33       |
| GPU        | 9.928      | 2.74    | 2.74       |

### Resolución: 20m

| Procesador | Tiempo (s) | Speedup | Eficiencia |
|------------|------------|---------|------------|
| CPU 1      | 7.888      | 1.00    | 1.00       |
| CPU 2      | 4.823      | 1.64    | 0.82       |
| CPU 4      | 2.923      | 2.70    | 0.67       |
| CPU 8      | 2.044      | 3.86    | 0.48       |
| CPU 12     | 1.770      | 4.46    | 0.37       |
| CPU 16     | 1.630      | 4.84    | 0.30       |
| GPU        | 1.520      | 5.19    | 5.19       |

### Resolución: 60m

| Procesador | Tiempo (s) | Speedup | Eficiencia |
|------------|------------|---------|------------|
| CPU 1      | 0.883      | 1.00    | 1.00       |
| CPU 2      | 0.919      | 0.96    | 0.48       |
| CPU 4      | 0.499      | 1.77    | 0.44       |
| CPU 8      | 0.339      | 2.60    | 0.33       |
| CPU 12     | 0.524      | 1.69    | 0.14       |
| CPU 16     | 0.642      | 1.38    | 0.09       |
| GPU        | 0.244      | 3.62    | 3.62       |

## Conclusiones principales

1. **Escalabilidad de CPU**: Para imágenes de alta resolución (10m), el rendimiento mejora significativamente al aumentar el número de núcleos, pero la eficiencia disminuye por encima de 8 núcleos.

2. **Rendimiento de GPU vs CPU**: La GPU supera a la CPU en resoluciones medias (20m) y bajas (60m), pero para imágenes de alta resolución (10m), la CPU con múltiples núcleos puede ser más eficiente.

3. **Cuellos de botella**: La decodificación de las imágenes (TTR NIR y TTR RED) constituye el principal cuello de botella en el procesamiento, seguido por el guardado de la imagen resultante.

4. **Comportamiento con resoluciones bajas**: En la resolución de 60m, se observa un comportamiento anómalo donde añadir más de 8 núcleos empeora el rendimiento debido a la sobrecarga de paralelización.

## Optimizaciones Implementadas

1. **Paralelismo en lectura**:
   - Lectura simultánea de ambas bandas (NIR y RED)
   - Decodificación paralela de tiles dentro de cada banda

2. **Optimización de memoria**:
   - Uso de estructuras de datos eficientes
   - Formatos de píxel optimizados para cada fase del procesamiento

3. **Cálculo NDVI eficiente**:
   - Implementación vectorizada
   - División del trabajo en bloques para mejor localidad de caché
   - Manejo especial de casos límite (división por cero)

4. **Colorización optimizada**:
   - Búsqueda binaria para asignación de colores
   - Interpolación eficiente entre puntos de color

5. **Guardado optimizado**:
   - Compresión PNG con nivel optimizado para velocidad
   - Escritura asíncrona al disco cuando es posible

## Notas Importantes

- El rendimiento puede variar según el sistema de archivos y velocidad del disco
- Para imágenes grandes, asegúrese de tener suficiente memoria RAM disponible
- Con GPU, la primera ejecución puede ser más lenta debido a la inicialización de CUDA
- Recomendamos usar 8 hilos como máximo para imágenes pequeñas (60m)
- Los archivos de salida se guardan en el directorio `go_jp2_direct/` (creado automáticamente)

## Solución de Problemas

### Problemas con nvJPEG2K
Si encuentra el error `cannot find -lnvjpeg2k`, verifique:
1. Que CUDA está correctamente instalado
   ```bash
   nvcc --version
   ```

2. Localice la biblioteca nvJPEG2K
   ```bash
   find /usr -name "*nvjpeg2k*"
   ```

3. Añada la ruta de bibliotecas CUDA a LD_LIBRARY_PATH:
   ```bash
   export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/local/cuda/lib64
   ```

4. Verifique que la biblioteca es accesible:
   ```bash
   ldconfig -p | grep nvjpeg2k
   ```

5. Si la biblioteca no está instalada, intente:
   ```bash
   # En sistemas Ubuntu
   sudo apt-get install cuda-nvjpeg2k
   
   # O instale el paquete completo CUDA extras
   sudo apt-get install cuda-toolkit-<version>-extras
   ```

### Problemas con OpenJPEG
Para errores de OpenJPEG, verifique la instalación con:
```bash
pkg-config --modversion libopenjp2
```

Si muestra errores, intente reinstalar:
```bash
sudo apt-get purge libopenjp2-7-dev
sudo apt-get install libopenjp2-7-dev
```

## Licencia

Este proyecto está disponible bajo la licencia MIT.