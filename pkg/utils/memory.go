package utils

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// PrintMemoryUsage outputs the current, total and OS memory usage
func PrintMemoryUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("Alloc = %v MiB", m.Alloc/1024/1024)
	fmt.Printf("\tTotalAlloc = %v MiB", m.TotalAlloc/1024/1024)
	fmt.Printf("\tSys = %v MiB", m.Sys/1024/1024)
	fmt.Printf("\tNumGC = %v MiB", m.NumGC)
	fmt.Printf("\tHeapAlloc = %v MiB", m.HeapAlloc/1024/1024)
	fmt.Printf("\tHeapSys = %v MiB", m.HeapSys/1024/1024)
	fmt.Printf("\tHeapIdle = %v MiB", m.HeapIdle/1024/1024)
	fmt.Printf("\tHeapInuse = %v MiB", m.HeapInuse/1024/1024)
	fmt.Printf("\tHeapReleased = %v MiB", m.HeapReleased/1024/1024)
	fmt.Printf("\tHeapObjects = %v MiB\n", m.HeapObjects)
}

// FreeMemory forces garbage collection and returns memory to OS
func FreeMemory() {
	runtime.GC()
	debug.FreeOSMemory()
}
