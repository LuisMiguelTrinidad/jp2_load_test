import numpy as np
from osgeo import gdal
import os
import time
import psutil
import datetime

# ================================================
# Configure single-threaded execution
# ================================================
gdal.SetConfigOption('GDAL_NUM_THREADS', '1')  # Restrict GDAL threads
os.environ['OMP_NUM_THREADS'] = '1'           # For NumPy/OpenMP
os.environ['OPENBLAS_NUM_THREADS'] = '1'      # For OpenBLAS backend

# ================================================
# Performance monitoring functions
# ================================================
def get_memory_usage():
    """Return current memory usage in MB"""
    process = psutil.Process(os.getpid())
    return process.memory_info().rss / (1024 * 1024)  # Convert to MB

def format_table(data, headers):
    """Format data as ASCII table"""
    # Calculate column widths
    col_widths = [max(len(str(row[i])) for row in [headers] + data) + 2 for i in range(len(headers))]
    
    # Create horizontal separator
    separator = '+' + '+'.join('-' * width for width in col_widths) + '+'
    
    # Create the table
    result = [separator]
    header_row = '|' + '|'.join(f' {h:<{w-2}} ' for h, w in zip(headers, col_widths)) + '|'
    result.append(header_row)
    result.append(separator)
    
    for row in data:
        result.append('|' + '|'.join(f' {str(cell):<{w-2}} ' for cell, w in zip(row, col_widths)) + '|')
    
    result.append(separator)
    return '\n'.join(result)

# ================================================
# NDVI Calculation Function
# ================================================
def calculate_ndvi(resolutions):
    # Performance data collection
    performance_data = []
    total_start_time = time.time()
    
    print(f"Starting NDVI calculation at {datetime.datetime.now()}")
    print(f"Initial memory usage: {get_memory_usage():.2f} MB")
    
    for res in resolutions:
        # Start timing for this resolution
        res_start_time = time.time()
        res_start_mem = get_memory_usage()
        
        print(f"\n{'='*50}")
        print(f"Processing resolution: {res}")
        print(f"{'='*50}")
        
        # Open input files
        print(f"[{res}] Opening input files...")
        open_start = time.time()
        open_start_mem = get_memory_usage()
        
        red_path = f'B04_{res}.jp2'
        nir_path = f'B08_{res}.jp2'
        
        red_ds = gdal.Open(red_path, gdal.GA_ReadOnly)
        nir_ds = gdal.Open(nir_path, gdal.GA_ReadOnly)
        
        open_end = time.time()
        open_end_mem = get_memory_usage()
        open_time = open_end - open_start
        open_mem = open_end_mem - open_start_mem
        
        print(f"[{res}] File opening completed in {open_time:.3f}s, memory change: {open_mem:.2f} MB")
        
        if not red_ds or not nir_ds:
            print(f"Skipping {res}: Missing input file(s)")
            continue

        # Validate raster dimensions
        if (red_ds.RasterXSize != nir_ds.RasterXSize) or \
           (red_ds.RasterYSize != nir_ds.RasterYSize):
            print(f"Skipping {res}: Dimension mismatch")
            continue
        
        print(f"[{res}] Raster dimensions: {red_ds.RasterXSize}x{red_ds.RasterYSize}")
        
        # Read raster bands as float32 arrays
        print(f"[{res}] Reading raster bands...")
        read_start = time.time()
        read_start_mem = get_memory_usage()
        
        red_band = red_ds.GetRasterBand(1).ReadAsArray().astype(np.float32)
        nir_band = nir_ds.GetRasterBand(1).ReadAsArray().astype(np.float32)
        
        read_end = time.time()
        read_end_mem = get_memory_usage()
        read_time = read_end - read_start
        read_mem = read_end_mem - read_start_mem
        
        print(f"[{res}] Band reading completed in {read_time:.3f}s, memory change: {read_mem:.2f} MB")
        print(f"[{res}] Array shapes: Red {red_band.shape}, NIR {nir_band.shape}")

        # Calculate NDVI
        print(f"[{res}] Calculating NDVI...")
        calc_start = time.time()
        calc_start_mem = get_memory_usage()
        
        numerator = nir_band - red_band
        denominator = nir_band + red_band
        ndvi = np.divide(numerator, denominator, 
                        where=denominator != 0, 
                        out=np.full_like(numerator, -9999, dtype=np.float32))
        
        calc_end = time.time()
        calc_end_mem = get_memory_usage()
        calc_time = calc_end - calc_start
        calc_mem = calc_end_mem - calc_start_mem
        
        print(f"[{res}] NDVI calculation completed in {calc_time:.3f}s, memory change: {calc_mem:.2f} MB")

        # Create output file
        print(f"[{res}] Creating output file...")
        write_start = time.time()
        write_start_mem = get_memory_usage()
        
        # Intenta con JP2, si no está disponible usa GeoTIFF
        driver = gdal.GetDriverByName('JP2')
        if driver is None:
            print(f"[{res}] JP2 driver no disponible, usando GeoTIFF...")
            driver = gdal.GetDriverByName('GTiff')
            out_path = f'NDVI_{res}.tif'
        else:
            out_path = f'NDVI_{res}.jp2'
        
        # Verifica que tengamos un driver válido
        if driver is None:
            print(f"[{res}] ERROR: No se pudo encontrar un driver de salida válido")
            continue
            
        out_ds = driver.Create(
            out_path,
            red_ds.RasterXSize,
            red_ds.RasterYSize,
            1,
            gdal.GDT_Float32
        )
        
        # Set spatial reference and write data
        out_ds.SetGeoTransform(red_ds.GetGeoTransform())
        out_ds.SetProjection(red_ds.GetProjection())
        out_band = out_ds.GetRasterBand(1)
        out_band.WriteArray(ndvi)
        out_band.SetNoDataValue(-9999)
        out_band.FlushCache()
        
        write_end = time.time()
        write_end_mem = get_memory_usage()
        write_time = write_end - write_start
        write_mem = write_end_mem - write_start_mem
        
        print(f"[{res}] File writing completed in {write_time:.3f}s, memory change: {write_mem:.2f} MB")

        # Cleanup resources
        print(f"[{res}] Cleaning up resources...")
        cleanup_start = time.time()
        red_ds = None
        nir_ds = None
        out_ds = None
        cleanup_end = time.time()
        cleanup_time = cleanup_end - cleanup_start
        
        # Calculate resolution total time
        res_end_time = time.time()
        res_total_time = res_end_time - res_start_time
        
        print(f"[{res}] Resource cleanup completed in {cleanup_time:.3f}s")
        print(f"[{res}] Total processing time: {res_total_time:.3f}s")
        print(f"Created: {out_path}")
        
        # Store performance data for this resolution
        performance_data.append([res, "File Opening", f"{open_time:.3f}", f"{open_mem:.2f}"])
        performance_data.append([res, "Band Reading", f"{read_time:.3f}", f"{read_mem:.2f}"])
        performance_data.append([res, "NDVI Calculation", f"{calc_time:.3f}", f"{calc_mem:.2f}"])
        performance_data.append([res, "File Writing", f"{write_time:.3f}", f"{write_mem:.2f}"])
        performance_data.append([res, "Cleanup", f"{cleanup_time:.3f}", "N/A"])
        performance_data.append([res, "TOTAL", f"{res_total_time:.3f}", "N/A"])

    # Calculate and show overall performance
    total_end_time = time.time()
    total_time = total_end_time - total_start_time
    
    print(f"\n{'='*50}")
    print(f"NDVI processing completed at {datetime.datetime.now()}")
    print(f"Total execution time: {total_time:.3f}s")
    print(f"Final memory usage: {get_memory_usage():.2f} MB")
    
    # Display performance table
    headers = ["Resolution", "Operation", "Time (sec)", "Memory Delta (MB)"]
    print(f"\n{'='*50}")
    print("PERFORMANCE ANALYSIS")
    print(f"{'='*50}")
    print(format_table(performance_data, headers))
    
    return performance_data

# ================================================
# Main Execution
# ================================================
if __name__ == "__main__":
    print(f"NDVI Processing started at: {datetime.datetime.now()}")
    print(f"Python process ID: {os.getpid()}")
    print(f"Initial memory usage: {get_memory_usage():.2f} MB")
    
    resolutions = ['10m', '20m', '60m']
    performance_data = calculate_ndvi(resolutions)
    
    print("NDVI processing complete. Outputs: NDVI_10m.jp2, NDVI_20m.jp2, NDVI_60m.jp2")