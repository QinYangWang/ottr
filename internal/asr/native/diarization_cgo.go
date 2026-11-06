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

// SpeakerSegment is one time range assigned to an anonymous speaker cluster.
type SpeakerSegment struct {
	StartMs int64
	EndMs   int64
	Speaker int
}

func Diarize(segmentation, embedding string, samples []float32) ([]SpeakerSegment, error) {
	if !loaded {
		return nil, ErrUnavailable
	}
	if len(samples) == 0 {
		return nil, errors.New("speaker diarization needs audio")
	}
	var result []SpeakerSegment
	withCString(segmentation, func(segmentation *C.char) {
		withCString(embedding, func(embedding *C.char) {
			var count C.int32_t
			ptr := C.sherpa_diarization_process(segmentation, embedding,
				(*C.float)(unsafe.Pointer(&samples[0])), C.int32_t(len(samples)), &count)
			if ptr == nil {
				return
			}
			defer C.sherpa_free(ptr)
			rows := unsafe.Slice((*C.OttrDiarizationSegment)(ptr), int(count))
			result = make([]SpeakerSegment, len(rows))
			for i, row := range rows {
				result[i] = SpeakerSegment{
					StartMs: int64(row.start * 1000), EndMs: int64(row.end * 1000), Speaker: int(row.speaker),
				}
			}
		})
	})
	if result == nil {
		return nil, errors.New("speaker diarization produced no result")
	}
	return result, nil
}
