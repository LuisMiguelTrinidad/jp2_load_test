package config

import (
	"image/color"
)

// NDVI gradient color points for visualization
var ndviGradientPoints = []struct {
	Value float64
	Color color.RGBA
}{
	{-1.0, color.RGBA{0, 0, 128, 255}},    // Dark blue (water/shadows)
	{-0.2, color.RGBA{65, 105, 225, 255}}, // Medium blue
	{0.0, color.RGBA{255, 0, 0, 255}},     // Red (soil/urban areas)
	{0.5, color.RGBA{255, 255, 0, 255}},   // Yellow (sparse vegetation)
	{1.0, color.RGBA{0, 128, 0, 255}},     // Green (dense vegetation)
}

// GetNDVIColor returns a color for the given NDVI value using optimized gradient lookup
func GetNDVIColor(ndviValue float64) color.RGBA {
	if ndviValue <= -1.0 {
		return ndviGradientPoints[0].Color
	}
	if ndviValue >= 1.0 {
		return ndviGradientPoints[len(ndviGradientPoints)-1].Color
	}

	// Binary search for the index
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

	// Special case for the last point
	if idx >= len(ndviGradientPoints)-1 {
		return ndviGradientPoints[len(ndviGradientPoints)-1].Color
	}

	// Efficient interpolation
	p1, p2 := ndviGradientPoints[idx], ndviGradientPoints[idx+1]
	t := (ndviValue - p1.Value) / (p2.Value - p1.Value)

	r := uint8(float64(p1.Color.R) + t*float64(p2.Color.R-p1.Color.R))
	g := uint8(float64(p1.Color.G) + t*float64(p2.Color.G-p1.Color.G))
	b := uint8(float64(p1.Color.B) + t*float64(p2.Color.B-p1.Color.B))

	return color.RGBA{r, g, b, 255}
}
