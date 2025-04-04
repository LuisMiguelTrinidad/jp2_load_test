package openjpeg

/*
#cgo CFLAGS: -I/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/include
#cgo LDFLAGS: -L/home/linuxbrew/.linuxbrew/Cellar/openjpeg/2.5.3/lib -lopenjp2
#include <openjpeg-2.5/openjpeg.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

// Callback functions for OpenJPEG
void error_callback(const char *msg, void *client_data) {
    fprintf(stderr, "[ERROR] %s", msg);
}

void warning_callback(const char *msg, void *client_data) {
    fprintf(stderr, "[WARNING] %s", msg);
}

void info_callback(const char *msg, void *client_data) {
//    fprintf(stdout, "[INFO] %s", msg);
}
*/
import "C"
import (
	"unsafe"
)

// GetCallbacks returns pointers to the callback functions for OpenJPEG
func GetCallbacks() (error, warning, info unsafe.Pointer) {
	return unsafe.Pointer(C.error_callback),
		unsafe.Pointer(C.warning_callback),
		unsafe.Pointer(C.info_callback)
}
