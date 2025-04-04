package metrics

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// PrintMetricsTable prints a table with performance metrics
func PrintMetricsTable(metricas []*Metrics) {
	// Bottleneck Analysis
	fmt.Println("┌ Bottleneck Analysis ────────────┬──────────────────┬──────────────────┬──────────────────┬──────────────────┬──────────────────┐")
	fmt.Printf("│ %-12s │ %-16s │ %-16s │ %-16s │ %-16s │ %-16s │ %-16s │\n",
		"Res",
		"TTR NIR",
		"TTR RED",
		"NDVI",
		"Color Proc.",
		"Saving",
		"Total")
	fmt.Println("├──────────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┼─────────┬────────┤")

	for _, m := range metricas {
		nirMag, nirUnit := getMagnitudeAndUnit(m.FileTimeNIR + m.DecodeTimeNIR)
		redMag, redUnit := getMagnitudeAndUnit(m.FileTimeRED + m.DecodeTimeRED)
		ndviMag, ndviUnit := getMagnitudeAndUnit(m.NDVITime)
		colorMag, colorUnit := getMagnitudeAndUnit(m.ColorTime)
		saveMag, saveUnit := getMagnitudeAndUnit(m.SaveTime)
		totalMag, totalUnit := getMagnitudeAndUnit(m.TotalTime)

		porcNIR := float64(m.FileTimeNIR+m.DecodeTimeNIR) / float64(m.TotalTime) * 100
		porcRED := float64(m.FileTimeRED+m.DecodeTimeRED) / float64(m.TotalTime) * 100
		porcNDVI := float64(m.NDVITime) / float64(m.TotalTime) * 100
		porcColor := float64(m.ColorTime) / float64(m.TotalTime) * 100
		porcSave := float64(m.SaveTime) / float64(m.TotalTime) * 100

		fmt.Printf("│ %-12s │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │ %s%-2s │ %s%% │\n",
			"",
			formatNumber(nirMag, 5), nirUnit, formatNumber(porcNIR, 5),
			formatNumber(redMag, 5), redUnit, formatNumber(porcRED, 5),
			formatNumber(ndviMag, 5), ndviUnit, formatNumber(porcNDVI, 5),
			formatNumber(colorMag, 5), colorUnit, formatNumber(porcColor, 5),
			formatNumber(saveMag, 5), saveUnit, formatNumber(porcSave, 5),
			formatNumber(totalMag, 5), totalUnit, "100.0") // Total is always 100%
	}
	fmt.Println("└──────────────┴─────────┴────────┴─────────┴────────┴─────────┴────────┴─────────┴────────┴─────────┴────────┴─────────┴────────┘")
	fmt.Println()

	// Image Reading Breakdown table
	fmt.Println("┌ Image Reading Breakdown ────────┬───────────┬───────────┬──────────┬──────────┐")
	fmt.Printf("│ %-12s │ %-16s │ %-9s │ %-9s │ %-8s │ %-8s │\n",
		"Res",
		"Uncovered Region",
		"NIR Tiles",
		"RED Tiles",
		"Total MP",
		"Img Size")
	fmt.Println("├──────────────┼─────────┬────────┼───────────┼───────────┼──────────┼──────────┤")

	for _, m := range metricas {
		porcNoData := float64(m.NoDataPixels) / float64(m.Pixels) * 100
		pixelesSinDatosMP := float64(m.NoDataPixels) / 1000000

		var totalPixelUnit string
		var totalPixelValue float64

		if m.Pixels >= 1000000 {
			totalPixelValue = float64(m.Pixels) / 1000000
			totalPixelUnit = "MP"
		} else if m.Pixels >= 1000 {
			totalPixelValue = float64(m.Pixels) / 1000
			totalPixelUnit = "KP"
		} else {
			totalPixelValue = float64(m.Pixels)
			totalPixelUnit = "P"
		}

		sizeMB := float64(m.ImageSize) / (1024 * 1024)

		fmt.Printf("│ %-12s │ %s%-2s │ %s%% │ %-3d tiles │ %-3d tiles │ %s %-2s │ %s MB │\n",
			"",
			formatNumber(pixelesSinDatosMP, 5), "MP", formatNumber(porcNoData, 5),
			m.NumTilesNIR,
			m.NumTilesRED,
			formatNumber(totalPixelValue, 5), totalPixelUnit,
			formatNumber(sizeMB, 5),
		)
	}
	fmt.Println("└──────────────┴─────────┴────────┴───────────┴───────────┴──────────┴──────────┘")
}

// PrintScalabilityAnalysis prints a table with scalability information
func PrintScalabilityAnalysis(metricas []*Metrics, groupByResolution bool) {
	fmt.Println("\n--- SCALABILITY ANALYSIS ---")

	// Group metrics by resolution
	metricsByResolution := make(map[string][]*Metrics)

	for _, m := range metricas {
		// Use resolution as the key
		baseRes := m.Resolution
		metricsByResolution[baseRes] = append(metricsByResolution[baseRes], m)
	}

	// Print scalability analysis for each resolution
	for res, ms := range metricsByResolution {
		if len(ms) > 0 {
			fmt.Printf("\nResolution: %s\n", res)
			fmt.Println("┌────────────┬────────────┬─────────┬────────────┐")
			fmt.Printf("│ %-10s │ %-10s │ %-7s │ %-10s │\n",
				"Processor", "Time (s)", "Speedup", "Efficiency")
			fmt.Println("├────────────┼────────────┼─────────┼────────────┤")

			// Find reference metric (1 core)
			var baseTime float64
			for _, m := range ms {
				if m.ProcessorType == "CPU" && m.NumThreads == 1 {
					baseTime = m.TotalTime.Seconds()
					break
				}
			}

			// If no single-core metric found, use the first one
			if baseTime == 0 && len(ms) > 0 {
				baseTime = ms[0].TotalTime.Seconds()
			}

			// Show metrics ordered by processor
			for _, m := range ms {
				time := m.TotalTime.Seconds()
				speedup := baseTime / time
				efficiency := 0.0

				if m.ProcessorType == "CPU" {
					efficiency = speedup / float64(m.NumThreads)
				} else {
					efficiency = speedup // For GPU we don't calculate efficiency
				}

				procLabel := fmt.Sprintf("%s %d", m.ProcessorType, m.NumThreads)
				if m.ProcessorType == "GPU" {
					procLabel = "GPU"
				}

				fmt.Printf("│ %-10s │ %10.3f │ %7.2f │ %10.2f │\n",
					procLabel, time, speedup, efficiency)
			}
			fmt.Println("└────────────┴────────────┴─────────┴────────────┘")
		}
	}
}

// getMagnitudeAndUnit returns the appropriate magnitude and unit for a duration
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

// formatNumber formats a number to display in the metrics table
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

// AggregateMetrics adds values from two metrics structures
// Used to accumulate metrics in multiple runs
func AggregateMetrics(accumulated, new *Metrics) {
	accumulated.TotalTime += new.TotalTime
	accumulated.ReadingTime += new.ReadingTime
	accumulated.NDVITime += new.NDVITime
	accumulated.ColorTime += new.ColorTime
	accumulated.SaveTime += new.SaveTime
	accumulated.FileTimeNIR += new.FileTimeNIR
	accumulated.DecodeTimeNIR += new.DecodeTimeNIR
	accumulated.FileTimeRED += new.FileTimeRED
	accumulated.DecodeTimeRED += new.DecodeTimeRED
	accumulated.NoDataPixels += new.NoDataPixels
	accumulated.NDVIMin = math.Min(accumulated.NDVIMin, new.NDVIMin)
	accumulated.NDVIMax = math.Max(accumulated.NDVIMax, new.NDVIMax)
	accumulated.NDVIAverage += new.NDVIAverage
}

// AverageMetrics calculates the average of accumulated metrics
func AverageMetrics(accumulated *Metrics, numRuns int) *Metrics {
	result := *accumulated // Copy all values

	// Divide times by number of runs
	result.TotalTime /= time.Duration(numRuns)
	result.ReadingTime /= time.Duration(numRuns)
	result.NDVITime /= time.Duration(numRuns)
	result.ColorTime /= time.Duration(numRuns)
	result.SaveTime /= time.Duration(numRuns)
	result.FileTimeNIR /= time.Duration(numRuns)
	result.DecodeTimeNIR /= time.Duration(numRuns)
	result.FileTimeRED /= time.Duration(numRuns)
	result.DecodeTimeRED /= time.Duration(numRuns)

	// Average numeric values
	result.NoDataPixels /= numRuns
	result.NDVIAverage /= float64(numRuns)

	// Min/Max and other static values remain the same

	return &result
}

// CopyMetrics creates a copy of the metrics
func CopyMetrics(m *Metrics) *Metrics {
	copy := *m // Copy all fields
	return &copy
}

// InitializeAccumulatedMetrics creates an initial structure for accumulating metrics
func InitializeAccumulatedMetrics(original *Metrics) *Metrics {
	accumulated := CopyMetrics(original)
	accumulated.NDVIMin = math.MaxFloat64  // To allow finding the real minimum
	accumulated.NDVIMax = -math.MaxFloat64 // To allow finding the real maximum
	return accumulated
}
