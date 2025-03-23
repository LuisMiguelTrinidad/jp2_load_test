import os
os.environ["GDAL_NUM_THREADS"] = "1"
os.environ["OMP_NUM_THREADS"] = "1"
os.environ["OPENBLAS_NUM_THREADS"] = "1"
os.environ["MKL_NUM_THREADS"] = "1"
# Solo configuraciones que soporta OpenJPEG
os.environ["VSI_CACHE"] = "TRUE"
os.environ["VSI_CACHE_SIZE"] = "32768"  # 32MB para caché VSI

import numpy as np
import time
import cv2
import sys
from datetime import datetime
from dataclasses import dataclass
import warnings
import logging

# Configuración básica del logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

# Usar GDAL directamente en lugar de rasterio
from osgeo import gdal
gdal.UseExceptions()  # Para mejor manejo de errores

# Diccionario para almacenar datasets ya abiertos (cache)
dataset_cache = {}

NDVI_GRADIENT = np.array([
    [-1.0, 0, 0, 128],
    [-0.2, 65, 105, 225],
    [0.0, 255, 0, 0],
    [0.5, 255, 255, 0],
    [1.0, 0, 128, 0]
], dtype=np.float32)

@dataclass
class ProcessingMetrics:
    resolucion: str
    timings: dict
    stats: dict
    pixels_per_sec: float
    total_time: float
    band_sizes: dict

def timed(fn):
    def wrapper(*args, **kwargs):
        start = time.perf_counter()
        result = fn(*args, **kwargs)
        return result, time.perf_counter() - start
    return wrapper

@timed
def read_band(path):
    logging.info(f"Leyendo banda: {path}")
    # Obtener tamaño de archivo
    file_size = os.path.getsize(path)

    # Eliminar las opciones que no soporta OpenJPEG
    # OpenJPEG no soporta las opciones que están generando advertencias

    # 1. Apertura del archivo (reutilizando datasets abiertos)
    global dataset_cache
    start_open = time.perf_counter()

    if path in dataset_cache:
        ds = dataset_cache[path]
        reused = True
        logging.info(f"  - Dataset encontrado en caché: {path}")
    else:
        try:
            # Abrir sin opciones especiales para OpenJPEG
            ds = gdal.Open(path)
            # Guardar en cache solo archivos <100MB para ahorrar memoria
            if file_size < 100*1024*1024:
                dataset_cache[path] = ds
            reused = False
        except Exception as e:
            logging.error(f"Error al abrir {path}: {e}")
            ds = gdal.Open(path)
            reused = False

    open_time = time.perf_counter() - start_open
    logging.info(f"  - Tiempo de apertura: {open_time:.4f}s")

    # 2. Lectura de los datos
    start_read = time.perf_counter()

    # Obtener información sobre la banda
    band = ds.GetRasterBand(1)
    width = ds.RasterXSize
    height = ds.RasterYSize
    logging.info(f"  - Dimensiones de la banda: {width}x{height}")

    # Leer los datos directamente - mucho más rápido que con rasterio
    data = band.ReadAsArray()

    # Normalización de valores a [0,1] - crítica para el cálculo posterior
    data_type = band.DataType
    logging.info(f"  - Tipo de datos de la banda: {gdal.GetDataTypeName(data_type)}")
    if data_type == gdal.GDT_UInt16:
        data = data.astype(np.float32) / 65535.0
        logging.info("  - Normalizando datos UInt16 a [0,1]")
    elif data_type == gdal.GDT_Byte:
        data = data.astype(np.float32) / 255.0
        logging.info("  - Normalizando datos Byte a [0,1]")
    else:
        # Para otros tipos, convertir a float32 sin normalizar
        data = data.astype(np.float32)
        logging.warning(f"  - Tipo de datos no normalizado: {gdal.GetDataTypeName(data_type)}")

    read_time = time.perf_counter() - start_read
    logging.info(f"  - Tiempo de lectura de datos: {read_time:.4f}s")

    # No cerramos el dataset si lo hemos cacheado
    if not reused and path not in dataset_cache:
        ds = None  # En GDAL, esto cierra el dataset
        logging.info(f"  - Dataset cerrado: {path}")

    # Información detallada sobre pasos de lectura
    detailed_timings = {
        'conversion': 0,  # No hay conversión explícita JP2 → JPG en Python
        'open_time': open_time,
        'read_time': read_time,
        'reused': reused
    }

    return (data, file_size, detailed_timings)

@timed
def compute_ndvi(nir, red):
    logging.info("Calculando NDVI...")
    # Optimización: preasignar arrays
    ndvi = np.zeros_like(nir)
    # Crear máscara para dividir con seguridad usando operaciones vectoriales
    sum_bands = np.add(nir, red, out=np.zeros_like(nir))
    valid_mask = sum_bands > 0

    # Calcular NDVI vectorialmente solo donde es válido
    np.subtract(nir, red, out=ndvi, where=valid_mask)
    np.divide(ndvi, sum_bands, out=ndvi, where=valid_mask)
    logging.info("  - Cálculo de NDVI completado.")
    return ndvi

@timed
def compute_statistics(ndvi):
    logging.info("Calculando estadísticas...")
    # Vector-optimized statistics calculation
    stats = {
        'min': float(np.min(ndvi)),
        'max': float(np.max(ndvi)),
        'mean': float(np.mean(ndvi)),
        'pixel_count': int(ndvi.size)
    }
    logging.info(f"  - Estadísticas calculadas: Min={stats['min']:.4f}, Max={stats['max']:.4f}, Mean={stats['mean']:.4f}, Pixels={stats['pixel_count']}")
    return stats

@timed
def save_grayscale(ndvi, resolution):
    logging.info(f"Guardando imagen en escala de grises: ndvi_{resolution}_gray.png")
    # CORREGIDO: Gestión de memoria segura
    # Crear una copia separada en vez de usar view()
    gray = np.zeros_like(ndvi, dtype=np.uint8)

    # Normalizar de forma segura [-1,1] a [0,255]
    normalized = np.clip((ndvi + 1.0) * 127.5, 0, 255).astype(np.uint8)

    # Guardar con compresión rápida y bloques más pequeños
    try:
        # Usar un tamaño de buffer de imagen más pequeño
        success = cv2.imwrite(f"ndvi_{resolution}_gray.png", normalized,
                               [cv2.IMWRITE_PNG_COMPRESSION, 0])
        if not success:
            logging.error("Error al guardar la imagen en escala de grises")
        else:
            logging.info("  - Imagen en escala de grises guardada exitosamente.")
    except Exception as e:
        logging.error(f"Error guardando imagen gris: {e}")

    # Limpieza explícita para liberar memoria
    del normalized
    return None

def save_color(ndvi, resolution):
    logging.info(f"Guardando imagen a color: ndvi_{resolution}_color.png")
    # Procesamiento de color en dos pasos para medir tiempo
    start_color_proc = time.perf_counter()

    # CORREGIDO: Usar procesamiento en lotes para imágenes grandes
    # Tamaño de lote para imágenes grandes
    batch_size = 1000000  # 1M píxeles por lote
    total_pixels = ndvi.size
    height, width = ndvi.shape

    # Preparar la imagen final
    bgr = np.zeros((height, width, 3), dtype=np.uint8)

    # Procesar por lotes para evitar problemas de memoria
    for start in range(0, total_pixels, batch_size):
        end = min(start + batch_size, total_pixels)

        # Obtener sección del array aplanado
        flat_section = ndvi.ravel()[start:end]

        # Buscar índices de forma optimizada
        indices = np.clip(
            np.searchsorted(NDVI_GRADIENT[:, 0], flat_section) - 1,
            0, len(NDVI_GRADIENT)-2
        )

        # Calcular factores de interpolación
        lower = NDVI_GRADIENT[indices, 0]
        upper = NDVI_GRADIENT[indices+1, 0]

        # Calcular factor t
        t = (flat_section - lower) / (upper - lower)
        t = t[:, np.newaxis]

        # Interpolación de colores
        lower_colors = NDVI_GRADIENT[indices, 1:]
        upper_colors = NDVI_GRADIENT[indices+1, 1:]

        # Calcular colores
        colors = ((1-t) * lower_colors + t * upper_colors).astype(np.uint8)

        # Colocar en la imagen final
        flat_bgr = bgr.reshape(-1, 3)
        flat_bgr[start:end] = colors[:, ::-1]  # RGB a BGR

    color_proc_time = time.perf_counter() - start_color_proc
    logging.info(f"  - Tiempo de procesamiento de color: {color_proc_time:.4f}s")

    # Guardar la imagen
    start_save = time.perf_counter()
    try:
        success = cv2.imwrite(f"ndvi_{resolution}_color.png", bgr,
                               [cv2.IMWRITE_PNG_COMPRESSION, 0])
        if not success:
            logging.error("Error al guardar la imagen a color")
        else:
            logging.info("  - Imagen a color guardada exitosamente.")
    except Exception as e:
        logging.error(f"Error guardando imagen color: {e}")

    save_time = time.perf_counter() - start_save
    logging.info(f"  - Tiempo de guardado de imagen a color: {save_time:.4f}s")

    # Limpieza explícita
    del bgr

    return (color_proc_time, save_time)

def process_resolution(nir_path, red_path, resolution):
    logging.info(f"---------- Procesando NDVI con RED {resolution} ----------")
    metrics = {
        'band_read': {'nir': {}, 'red': {}},
        'ndvi_steps': {},
        'save_times': {},
        'color_processing': 0,
    }
    band_sizes = {'nir': 0, 'red': 0}
    start_total = time.perf_counter()

    # [1/5] Leyendo imágenes...
    logging.info("[1/5] Leyendo imágenes...")
    start_nir = time.perf_counter()
    nir_result = read_band(nir_path)
    nir = nir_result[0][0]  # La imagen
    band_sizes['nir'] = nir_result[0][1]  # El tamaño del archivo
    nir_detailed = nir_result[0][2]  # Los tiempos detallados
    metrics['band_read']['nir'] = nir_result[1]  # El tiempo total
    logging.info(f"  Tiempo total lectura NIR: {metrics['band_read']['nir']:.4f}s")

    start_red = time.perf_counter()
    red_result = read_band(red_path)
    red = red_result[0][0]  # La imagen
    band_sizes['red'] = red_result[0][1]  # El tamaño del archivo
    red_detailed = red_result[0][2]  # Los tiempos detallados
    metrics['band_read']['red'] = red_result[1]  # El tiempo total
    logging.info(f"  Tiempo total lectura RED: {metrics['band_read']['red']:.4f}s")

    # Mostrar dimensiones y tamaños
    height, width = nir.shape
    pixel_count = width * height
    logging.info(f"Dimensiones NIR: {width}x{height} ({pixel_count} px)")
    logging.info(f"Tamaños de archivo - NIR: {band_sizes['nir']} bytes, RED: {band_sizes['red']} bytes")
    logging.info(f"Tiempos de carga - NIR: {metrics['band_read']['nir']:.4f}s, RED: {metrics['band_read']['red']:.4f}s")

    # [2/5] Calculando NDVI...
    logging.info("[2/5] Calculando NDVI...")
    start_ndvi = time.perf_counter()
    ndvi_result, ndvi_time = compute_ndvi(nir, red)
    metrics['ndvi_steps']['calculation'] = ndvi_time

    # Para imágenes grandes, podemos liberar memoria después de calcular NDVI
    del nir
    del red

    # Calcular estadísticas
    stats_result, stats_time = compute_statistics(ndvi_result)
    metrics['ndvi_steps']['statistics'] = stats_time

    # Mostrar progreso durante el cálculo
    total_ndvi_time = ndvi_time + stats_time
    logging.info(f"NDVI calculado: {total_ndvi_time:.4f}s ({pixel_count/total_ndvi_time:.2f} px/s)")

    # [3/5] Guardar imagen en escala de grises...
    logging.info("[3/5] Guardando imagen en escala de grises...")
    _, gray_save_time = save_grayscale(ndvi_result, resolution)
    metrics['save_times']['grayscale'] = gray_save_time
    logging.info(f"  Tiempo de guardado (gris): {gray_save_time:.4f}s")

    # [4/5] Procesando imagen a color...
    logging.info("[4/5] Procesando imagen a color...")
    color_proc_time, color_save_time = save_color(ndvi_result, resolution)
    metrics['color_processing'] = color_proc_time
    logging.info(f"  Tiempo de procesamiento de color: {color_proc_time:.4f}s")

    # [5/5] Guardando imagen en color...
    logging.info("[5/5] Guardando imagen en color...")
    metrics['save_times']['color'] = color_save_time
    logging.info(f"  Tiempo de guardado (color): {color_save_time:.4f}s")

    # Calcular tiempo total
    total_time = time.perf_counter() - start_total
    logging.info(f"Tiempo total de procesamiento para {resolution}: {total_time:.4f}s")

    return ProcessingMetrics(
        resolucion=resolution,
        timings={
            'band_read': {
                'nir': metrics['band_read']['nir'],
                'red': metrics['band_read']['red'],
                'nir_detailed': nir_detailed,
                'red_detailed': red_detailed
            },
            'ndvi_steps': metrics['ndvi_steps'],
            'save_times': metrics['save_times'],
            'color_processing': metrics['color_processing']
        },
        stats=stats_result,
        pixels_per_sec=stats_result['pixel_count'] / metrics['ndvi_steps']['calculation'],
        total_time=total_time,
        band_sizes=band_sizes
    )

def imprimir_analisis_rendimiento(resultados):
    print("\n\n======== ANÁLISIS DETALLADO DE RENDIMIENTO ========")

    # Imprimir tabla comparativa general
    print("\n== Métricas Generales ==")
    print("%-10s | %-15s | %-15s | %-15s | %-15s" % (
        "Resolución", "Tiempo Total", "Tiempo NDVI", "Píx/segundo", "Tam. Imagen"))

    print("-" * 80)

    for r in resultados:
        ndvi_time = r.timings['ndvi_steps']['calculation'] + r.timings['ndvi_steps']['statistics']
        print("%-10s | %-15.4fs | %-15.4fs | %-15.2f | %-15s" % (
            r.resolucion,
            r.total_time,
            ndvi_time,
            r.pixels_per_sec,
            f"{(r.band_sizes['nir'] + r.band_sizes['red'])/1048576:.1f} MB"))

    # Análisis de cuellos de botella
    print("\n== Tiempos por Stem (segundos) ==")
    print("%-10s | %-15s | %-15s | %-15s | %-15s | %-15s | %-15s | %-15s" % (
        "Resolución", "Lectura NIR", "Lectura RED", "Cálculo NDVI", "Estadísticas", "Guardado (Gris)", "Proc. Color", "Guardado (Color)"))
    print("-" * 115)
    for r in resultados:
        print("%-10s | %-15.4f | %-15.4f | %-15.4f | %-15.4f | %-15.4f | %-15.4f | %-15.4f" % (
            r.resolucion,
            r.timings['band_read']['nir'],
            r.timings['band_read']['red'],
            r.timings['ndvi_steps']['calculation'],
            r.timings['ndvi_steps']['statistics'],
            r.timings['save_times']['grayscale'],
            r.timings['color_processing'],
            r.timings['save_times']['color']))

    # Análisis de cuellos de botella (agrupado)
    print("\n== Análisis de Cuellos de Botella (Agrupado) ==")
    print("%-10s | %-15s | %-15s | %-15s | %-15s" % (
        "Resolución", "Lectura", "NDVI", "Proc. Color", "Guardado"))

    print("-" * 80)

    for r in resultados:
        read_total = r.timings['band_read']['nir'] + r.timings['band_read']['red']
        ndvi_total = r.timings['ndvi_steps']['calculation'] + r.timings['ndvi_steps']['statistics']
        color_proc = r.timings['color_processing']
        save_total = r.timings['save_times']['grayscale'] + r.timings['save_times']['color']

        print("%-10s | %-15.2fs | %-15.2fs | %-15.2fs | %-15.2fs" % (
            r.resolucion,
            read_total,
            ndvi_total,
            color_proc,
            save_total))

    # Detallar tiempos de lectura
    print("\n== Desglose de Lectura de Imágenes ==")
    print("%-10s | %-15s | %-15s | %-15s" % (
        "Resolución", "Apertura", "Lectura", "Reutilizado"))

    print("-" * 65)

    for r in resultados:
        # Promedios de NIR y RED
        open_nir = r.timings['band_read']['nir_detailed']['open_time']
        read_nir = r.timings['band_read']['nir_detailed']['read_time']
        reused_nir = r.timings['band_read']['nir_detailed']['reused']
        open_red = r.timings['band_read']['red_detailed']['open_time']
        read_red = r.timings['band_read']['red_detailed']['read_time']
        reused_red = r.timings['band_read']['red_detailed']['reused']

        print("%-10s | %-15.4fs | %-15.4fs | NIR: %-5s, RED: %-5s" % (
            r.resolucion,
            (open_nir + open_red) / 2,
            (read_nir + read_red) / 2,
            reused_nir,
            reused_red))

    # Estadísticas NDVI mínimas
    print("\n== Datos NDVI ==")
    print("%-10s | %-10s | %-10s | %-10s" % (
        "Resolución", "Mínimo", "Máximo", "Promedio"))

    print("-" * 45)

    for r in resultados:
        print("%-10s | %-10.4f | %-10.4f | %-10.4f" % (
            r.resolucion,
            r.stats['min'],
            r.stats['max'],
            r.stats['mean']))

    print("=" * 115)

def main():
    # Configuración global específica para JP2
    # OpenJPEG no admite estas opciones, las quitamos

    # Intentar determinar y usar el driver JP2 más rápido disponible
    drivers = ['JP2OpenJPEG', 'JP2KAK', 'JP2ECW', 'JP2MrSID', 'JPEG2000']
    for driver in drivers:
        if gdal.GetDriverByName(driver):
            logging.info(f"Usando driver JP2: {driver}")
            # No establecer GDAL_DEFAULT_JPEG2000_DRIVER ya que puede causar problemas
            break

    try:
        configs = [
            ("./B08_10m.jp2", "./B04_10m.jp2", "10m"),
            ("./B08_20m.jp2", "./B04_20m.jp2", "20m"),
            ("./B08_60m.jp2", "./B04_60m.jp2", "60m"),
        ]

        results = []
        for cfg in configs:
            metrics = process_resolution(*cfg)
            results.append(metrics)

        # Imprimir análisis de rendimiento
        imprimir_analisis_rendimiento(results)
    finally:
        # CORREGIDO: Cierre adecuado de datasets
        for key in list(dataset_cache.keys()):
            try:
                dataset_cache[key] = None
            except Exception as e:
                logging.error(f"Error al liberar dataset: {e}")
                pass
        dataset_cache.clear()
        logging.info("Cierre de datasets completado.")

if __name__ == "__main__":
    main()