package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/luismi/jp2_processing/pkg/jp2"
	"github.com/luismi/jp2_processing/pkg/jp2/cpu"
	"github.com/luismi/jp2_processing/pkg/jp2/gpu"
	"github.com/luismi/jp2_processing/pkg/metrics"
	"github.com/luismi/jp2_processing/pkg/ndvi"
	"github.com/luismi/jp2_processing/pkg/utils"
)

// Update the threads parameter to accept a list of configurations
var (
	// Command-line flags
	runGPU     = flag.Bool("gpu", false, "Use GPU for processing")
	runCPU     = flag.Bool("cpu", true, "Use CPU for processing")
	nirFile    = flag.String("nir", "", "Path to NIR band JP2 file")
	redFile    = flag.String("red", "", "Path to RED band JP2 file")
	threads    = flag.String("threads", "2,4,8,12,16", "Comma-separated list of thread configurations to use for CPU processing")
	iterations = flag.Int("iter", 1, "Number of iterations to run")
)

func parseThreads(threadsFlag string) []int {
	var threadConfigs []int
	for _, t := range strings.Split(threadsFlag, ",") {
		threads, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			fmt.Printf("Invalid thread configuration: %s\n", t)
			os.Exit(1)
		}
		threadConfigs = append(threadConfigs, threads)
	}
	return threadConfigs
}

func main() {
	// Parse command-line flags
	flag.Parse()

	// Parse thread configurations
	threadConfigs := parseThreads(*threads)

	// Validate required parameters
	if *nirFile == "" || *redFile == "" {
		fmt.Println("Error: NIR and RED band files must be specified")
		flag.Usage()
		os.Exit(1)
	}

	// Check if at least one processor is selected
	if !*runCPU && !*runGPU {
		fmt.Println("Error: Must use at least one processor (CPU or GPU)")
		flag.Usage()
		os.Exit(1)
	}

	// Print configuration information
	fmt.Println("NDVI Benchmark Configuration:")
	fmt.Printf("  NIR Band: %s\n", *nirFile)
	fmt.Printf("  RED Band: %s\n", *redFile)
	fmt.Printf("  Iterations: %d\n", *iterations)
	fmt.Printf("  Thread Configurations: %v\n", threadConfigs)

	// Collect metrics for all runs
	var allMetrics []*metrics.Metrics

	// Run CPU benchmark for each thread configuration if selected
	if *runCPU {
		for _, threadCount := range threadConfigs {
			fmt.Printf("\nRunning CPU benchmark with %d threads...\n", threadCount)
			cpuMetrics := runBenchmark("CPU", *nirFile, *redFile, threadCount, *iterations)
			allMetrics = append(allMetrics, cpuMetrics)
		}
	}

	// Run GPU benchmark if selected and GPU is available
	if *runGPU {
		fmt.Println("\nRunning GPU benchmark...")
		gpuMetrics := runBenchmark("GPU", *nirFile, *redFile, 1, *iterations)
		allMetrics = append(allMetrics, gpuMetrics)
	}

	// Print metrics results
	if len(allMetrics) > 0 {
		fmt.Println("\n=== Benchmark Results ===")
		metrics.PrintMetricsTable(allMetrics)

		metrics.PrintScalabilityAnalysis(allMetrics, true)
	}
}

// runBenchmark runs the NDVI benchmark with the specified processor type and settings
func runBenchmark(processorType string, nirFilePath, redFilePath string, numThreads, iterations int) *metrics.Metrics {
	// Create appropriate reader and writer based on processor type
	var reader jp2.Reader
	var writer jp2.Writer

	if processorType == "CPU" {
		reader = cpu.NewReader()
		writer = cpu.NewWriter("./go_jp2_direct/output_cpu.jp2") // Pass the required string argument
	} else if processorType == "GPU" {
		reader = gpu.NewReader()
		writer = gpu.NewWriter("./go_jp2_direct/output_gpu.jp2") // Pass the required string argument
	} else {
		fmt.Printf("Error: Unknown processor type: %s\n", processorType)
		os.Exit(1)
	}

	// Run multiple iterations to get average metrics
	var accumulatedMetrics *metrics.Metrics

	for i := 0; i < iterations; i++ {
		fmt.Printf("Running iteration %d/%d...\n", i+1, iterations)

		// Create metrics collector
		collector := metrics.NewCollector(processorType, numThreads)

		// Start timing
		startTime := collector.StartTiming()

		// Read NIR and RED bands
		fmt.Printf("Reading NIR band: %s\n", nirFilePath)
		nirBand, err := reader.Read(nirFilePath, numThreads)
		if err != nil {
			fmt.Printf("Error reading NIR band: %v\n", err)
			os.Exit(1)
		}
		defer nirBand.Free()

		fmt.Printf("Reading RED band: %s\n", redFilePath)
		redBand, err := reader.Read(redFilePath, numThreads)
		if err != nil {
			fmt.Printf("Error reading RED band: %v\n", err)
			os.Exit(1)
		}
		defer redBand.Free()

		// Record band read metrics
		collector.SetBandReadMetrics(&nirBand.Metrics, &redBand.Metrics)
		collector.SetNumTiles(nirBand.Metrics.NumTiles, redBand.Metrics.NumTiles)

		// Calculate NDVI
		fmt.Println("Calculating NDVI...")
		startNDVI := time.Now()
		ndviMetrics, ndviData, err := ndvi.Calculate(nirBand, redBand, numThreads)
		if err != nil {
			fmt.Printf("Error calculating NDVI: %v\n", err)
			os.Exit(1)
		}
		ndviTime := time.Since(startNDVI)
		collector.SetNDVIMetrics(ndviMetrics, ndviTime)

		// Colorize NDVI values
		fmt.Println("Colorizing NDVI...")
		startColor := time.Now()
		colorMetrics, ndviColorImg := ndvi.Colorize(ndviData, nirBand.Image.Width, nirBand.Image.Height, numThreads)
		colorTime := time.Since(startColor)
		collector.SetColorMetrics(colorMetrics, colorTime)

		// Save colorized image
		fmt.Println("Saving colorized image...")
		saveTime, err := writer.Write(ndviColorImg, "./go_jp2_direct/output_path.jp2", numThreads) // Add the required string argument
		if err != nil {
			fmt.Printf("Error saving image: %v\n", err)
			os.Exit(1)
		}
		collector.SetSaveTime(saveTime)

		// Stop timing
		collector.StopTiming(startTime)

		// Get metrics for this iteration
		iterationMetrics := collector.GetMetrics()

		// Accumulate metrics
		if i == 0 {
			accumulatedMetrics = metrics.InitializeAccumulatedMetrics(iterationMetrics)
		} else {
			metrics.AggregateMetrics(accumulatedMetrics, iterationMetrics)
		}

		// Force garbage collection between iterations
		ndviData = nil
		utils.FreeMemory()
	}

	// Calculate average metrics
	averageMetrics := metrics.AverageMetrics(accumulatedMetrics, iterations)

	return averageMetrics
}
