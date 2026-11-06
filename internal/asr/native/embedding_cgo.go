//go:build cgo

package native

/*
#include "sherpa_shim.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"unsafe"
)

func SpeakerEmbedding(model string, samples []float32) ([]float32, error) {
	if !loaded {
		return nil, ErrUnavailable
	}
	if len(samples) == 0 {
		return nil, errors.New("speaker embedding needs audio")
	}
	var result []float32
	withCString(model, func(model *C.char) {
		var dim C.int32_t
		ptr := C.sherpa_speaker_embedding(model, (*C.float)(unsafe.Pointer(&samples[0])), C.int32_t(len(samples)), &dim)
		if ptr == nil {
			return
		}
		defer C.sherpa_free(unsafe.Pointer(ptr))
		values := unsafe.Slice(ptr, int(dim))
		result = make([]float32, len(values))
		for i, value := range values {
			result[i] = float32(value)
		}
	})
	if len(result) == 0 {
		return nil, errors.New("speaker embedding is not ready")
	}
	return result, nil
}
