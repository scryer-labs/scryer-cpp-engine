//go:build cgo && darwin

package main

/*
#include <stdlib.h>
#include "scryer_engine_v1.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"sync"
	"unsafe"

	"github.com/scryer-labs/scryer-cpp-engine/internal/engine"
)

var active struct {
	sync.Mutex
	id     string
	cancel context.CancelFunc
}

func result(status C.int32_t, payload []byte) C.scryer_result_v1 {
	return C.scryer_result_v1{
		status:   status,
		response: C.scryer_buffer_v1{data: (*C.char)(C.CBytes(payload)), size: C.size_t(len(payload))},
	}
}

func failure(status C.int32_t, message string) C.scryer_result_v1 {
	payload, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{message})
	return result(status, payload)
}

func readView(view C.scryer_view_v1, limit C.size_t) ([]byte, bool) {
	if view.data == nil || view.size == 0 || view.size > limit {
		return nil, false
	}
	return C.GoBytes(unsafe.Pointer(view.data), C.int(view.size)), true
}

//export execute_v1
func execute_v1(input C.scryer_view_v1) C.scryer_result_v1 {
	data, ok := readView(input, engine.MaxRequestBytes)
	if !ok {
		return failure(C.SCRYER_ERROR_V1, "invalid request buffer")
	}
	request, err := engine.ParseRequest(data)
	if err != nil {
		return failure(C.SCRYER_ERROR_V1, err.Error())
	}

	active.Lock()
	if active.cancel != nil {
		active.Unlock()
		return failure(C.SCRYER_BUSY_V1, "C++ engine already has an active task")
	}
	ctx, cancel := context.WithCancel(context.Background())
	active.id, active.cancel = request.ID, cancel
	active.Unlock()

	output, err := request.Execute(ctx)
	active.Lock()
	cancelled := ctx.Err() != nil
	active.id, active.cancel = "", nil
	active.Unlock()
	cancel()
	if cancelled {
		return failure(C.SCRYER_CANCELLED_V1, "C++ task cancelled")
	}

	if err != nil {
		return failure(C.SCRYER_ERROR_V1, err.Error())
	}
	return result(C.SCRYER_OK_V1, output)
}

//export cancel_v1
func cancel_v1(input C.scryer_view_v1) C.bool {
	id, ok := readView(input, engine.MaxRequestIDBytes)
	if !ok {
		return false
	}
	active.Lock()
	defer active.Unlock()
	if active.cancel == nil || active.id != string(id) {
		return false
	}
	active.cancel()
	return true
}

//export release_buffer_v1
func release_buffer_v1(buffer C.scryer_buffer_v1) {
	C.free(unsafe.Pointer(buffer.data))
}

func main() {}
