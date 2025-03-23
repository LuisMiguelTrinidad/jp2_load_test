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
| 10m CPU 1c   | 12.69s (40.05%)  | 12.14s (38.30%)  | 235.7ms (0.744%) | 852.4ms (2.690%) | 5.773s (18.22%)  | 31.69s (100.0%)  |
| 10m CPU 2c   | 6.707s (34.52%)  | 6.405s (32.97%)  | 111.6ms (0.575%) | 438.5ms (2.258%) | 5.763s (29.67%)  | 19.43s (100.0%)  |
| 10m CPU 4c   | 3.738s (27.94%)  | 3.534s (26.41%)  | 88.72ms (0.663%) | 245.3ms (1.833%) | 5.774s (43.15%)  | 13.38s (100.0%)  |
| 10m CPU 6c   | 2.758s (24.09%)  | 2.627s (22.94%)  | 99.96ms (0.873%) | 189.1ms (1.651%) | 5.776s (50.44%)  | 11.45s (100.0%)  |
| 10m CPU 8c   | 2.403s (22.42%)  | 2.236s (20.86%)  | 107.4ms (1.002%) | 188.0ms (1.755%) | 5.780s (53.95%)  | 10.71s (100.0%)  |
| 10m CPU 10c  | 2.230s (21.50%)  | 2.070s (19.96%)  | 111.7ms (1.077%) | 187.8ms (1.811%) | 5.770s (55.64%)  | 10.37s (100.0%)  |
| 10m CPU 12c  | 2.078s (20.61%)  | 1.940s (19.24%)  | 113.4ms (1.125%) | 177.5ms (1.761%) | 5.773s (57.25%)  | 10.08s (100.0%)  |
| 10m CPU 14c  | 1.945s (19.81%)  | 1.825s (18.60%)  | 116.5ms (1.187%) | 164.6ms (1.677%) | 5.763s (58.72%)  | 9.816s (100.0%)  |
| 10m CPU 16c  | 1.889s (19.47%)  | 1.760s (18.14%)  | 117.7ms (1.213%) | 155.3ms (1.601%) | 5.780s (59.57%)  | 9.703s (100.0%)  |
| 10m GPU      | 2.608s (21.74%)  | 2.595s (21.62%)  | 167.8ms (1.398%) | 841.4ms (7.013%) | 5.774s (48.13%)  | 12.00s (100.0%)  |
| 20m CPU 1c   | 3.583s (39.72%)  | 3.614s (40.07%)  | 44.05ms (0.488%) | 220.0ms (2.439%) | 1.559s (17.28%)  | 9.021s (100.0%)  |
| 20m CPU 2c   | 2.143s (35.71%)  | 2.153s (35.87%)  | 26.31ms (0.438%) | 114.9ms (1.915%) | 1.564s (26.05%)  | 6.001s (100.0%)  |
| 20m CPU 4c   | 1.239s (30.09%)  | 1.226s (29.78%)  | 22.87ms (0.556%) | 69.91ms (1.699%) | 1.557s (37.84%)  | 4.115s (100.0%)  |
| 20m CPU 6c   | 951.1ms (26.97%) | 934.7ms (26.51%) | 24.28ms (0.689%) | 60.76ms (1.723%) | 1.554s (44.08%)  | 3.526s (100.0%)  |
| 20m CPU 8c   | 829.8ms (25.37%) | 799.4ms (24.44%) | 26.72ms (0.817%) | 54.24ms (1.658%) | 1.560s (47.68%)  | 3.271s (100.0%)  |
| 20m CPU 10c  | 755.1ms (24.23%) | 725.3ms (23.27%) | 27.45ms (0.881%) | 51.56ms (1.654%) | 1.556s (49.92%)  | 3.117s (100.0%)  |
| 20m CPU 12c  | 695.3ms (23.16%) | 674.1ms (22.45%) | 28.30ms (0.942%) | 46.65ms (1.554%) | 1.557s (51.85%)  | 3.003s (100.0%)  |
| 20m CPU 14c  | 646.1ms (22.24%) | 629.7ms (21.68%) | 29.27ms (1.008%) | 41.89ms (1.442%) | 1.556s (53.57%)  | 2.905s (100.0%)  |
| 20m CPU 16c  | 643.5ms (22.35%) | 606.7ms (21.07%) | 29.14ms (1.012%) | 40.22ms (1.397%) | 1.558s (54.12%)  | 2.879s (100.0%)  |
| 20m GPU      | 220.7ms (9.716%) | 223.8ms (9.852%) | 43.11ms (1.898%) | 219.3ms (9.654%) | 1.560s (68.68%)  | 2.272s (100.0%)  |
| 60m CPU 1c   | 397.4ms (39.06%) | 400.0ms (39.31%) | 5.871ms (0.577%) | 29.69ms (2.918%) | 184.3ms (18.11%) | 1.018s (100.0%)  |
| 60m CPU 2c   | 434.8ms (40.31%) | 435.3ms (40.36%) | 3.540ms (0.328%) | 16.02ms (1.486%) | 188.4ms (17.47%) | 1.078s (100.0%)  |
| 60m CPU 4c   | 226.7ms (34.83%) | 221.0ms (33.96%) | 2.487ms (0.382%) | 9.996ms (1.536%) | 190.1ms (29.22%) | 650.7ms (100.0%) |
| 60m CPU 6c   | 152.4ms (30.32%) | 148.6ms (29.57%) | 2.516ms (0.501%) | 8.979ms (1.787%) | 189.4ms (37.70%) | 502.5ms (100.0%) |
| 60m CPU 8c   | 135.7ms (28.59%) | 140.0ms (29.51%) | 2.651ms (0.559%) | 6.740ms (1.421%) | 188.7ms (39.77%) | 474.4ms (100.0%) |
| 60m CPU 10c  | 184.4ms (31.80%) | 186.8ms (32.20%) | 2.712ms (0.468%) | 6.411ms (1.105%) | 198.9ms (34.29%) | 580.0ms (100.0%) |
| 60m CPU 12c  | 247.7ms (36.07%) | 235.8ms (34.34%) | 2.681ms (0.390%) | 5.830ms (0.849%) | 193.7ms (28.21%) | 686.6ms (100.0%) |
| 60m CPU 14c  | 276.1ms (36.39%) | 276.2ms (36.40%) | 2.755ms (0.363%) | 5.104ms (0.673%) | 197.6ms (26.04%) | 758.7ms (100.0%) |
| 60m CPU 16c  | 298.5ms (37.58%) | 294.2ms (37.03%) | 2.798ms (0.352%) | 4.666ms (0.587%) | 193.1ms (24.31%) | 794.5ms (100.0%) |
| 60m GPU      | 47.97ms (14.81%) | 47.87ms (14.78%) | 5.533ms (1.708%) | 28.09ms (8.672%) | 192.8ms (59.52%) | 323.9ms (100.0%) |
### Desglose de Lectura de Imágenes

| Resolución   | Uncovered Region | Tiles NIR    | Tiles RED    | Total MP     | Img Size    |
|--------------|------------------|--------------|--------------|--------------|-------------|
| 10m CPU 1c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 2c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 4c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 6c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 8c   | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 10c  | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 12c  | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 14c  | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m CPU 16c  | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 10m GPU      | 0.000MP (0.000%) | 121 tiles    | 121 tiles    | 120.6 MP     | 459.9 MB    |
| 20m CPU 1c   | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m CPU 2c   | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m CPU 4c   | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m CPU 6c   | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m CPU 8c   | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m CPU 10c  | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m CPU 12c  | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m CPU 14c  | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m CPU 16c  | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 20m GPU      | 0.000MP (0.000%) | 81 tiles     | 81 tiles     | 30.14 MP     | 115.0 MB    |
| 60m CPU 1c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 2c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 4c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 6c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 8c   | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 10c  | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 12c  | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 14c  | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m CPU 16c  | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |
| 60m GPU      | 0.000MP (0.000%) | 100 tiles    | 100 tiles    | 3.349 MP     | 12.78 MB    |

## Análisis de Escalabilidad

### Resolución: 10m

| Procesador | Tiempo (s) | Speedup | Eficiencia |
|------------|------------|---------|------------|
| CPU 1      | 31.689     | 1.00    | 1.00       |
| CPU 2      | 19.425     | 1.63    | 0.82       |
| CPU 4      | 13.380     | 2.37    | 0.59       |
| CPU 6      | 11.451     | 2.77    | 0.46       |
| CPU 8      | 10.714     | 2.96    | 0.37       |
| CPU 10     | 10.370     | 3.06    | 0.31       |
| CPU 12     | 10.083     | 3.14    | 0.26       |
| CPU 14     | 9.816      | 3.23    | 0.23       |
| CPU 16     | 9.703      | 3.27    | 0.20       |
| GPU        | 11.998     | 2.64    | 2.64       |

### Resolución: 20m

| Procesador | Tiempo (s) | Speedup | Eficiencia |
|------------|------------|---------|------------|
| CPU 1      | 9.021      | 1.00    | 1.00       |
| CPU 2      | 6.001      | 1.50    | 0.75       |
| CPU 4      | 4.115      | 2.19    | 0.55       |
| CPU 6      | 3.526      | 2.56    | 0.43       |
| CPU 8      | 3.271      | 2.76    | 0.34       |
| CPU 10     | 3.117      | 2.89    | 0.29       |
| CPU 12     | 3.003      | 3.00    | 0.25       |
| CPU 14     | 2.905      | 3.11    | 0.22       |
| CPU 16     | 2.879      | 3.13    | 0.20       |
| GPU        | 2.272      | 3.97    | 3.97       |

### Resolución: 60m

| Procesador | Tiempo (s) | Speedup | Eficiencia |
|------------|------------|---------|------------|
| CPU 1      | 1.018      | 1.00    | 1.00       |
| CPU 2      | 1.078      | 0.94    | 0.47       |
| CPU 4      | 0.651      | 1.56    | 0.39       |
| CPU 6      | 0.502      | 2.03    | 0.34       |
| CPU 8      | 0.474      | 2.14    | 0.27       |
| CPU 10     | 0.580      | 1.75    | 0.18       |
| CPU 12     | 0.687      | 1.48    | 0.12       |
| CPU 14     | 0.759      | 1.34    | 0.10       |
| CPU 16     | 0.794      | 1.28    | 0.08       |
| GPU        | 0.324      | 3.14    | 3.14       |

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