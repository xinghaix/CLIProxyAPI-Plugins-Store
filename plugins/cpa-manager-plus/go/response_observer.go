package main

import (
	"encoding/json"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// Observers always return an empty modification: malformed/unsupported telemetry
// must never fail the model request, replace headers, or drop a streaming chunk.
func handleResponseObservation(method string, raw []byte) ([]byte, error) {
	runtime := currentRuntime()
	const maxObservationEnvelope = 8 << 20
	canObserve := runtime != nil && len(raw) <= maxObservationEnvelope
	if runtime != nil && len(raw) > maxObservationEnvelope {
		runtime.DisableResponseObserver("response observation envelope exceeds 8 MiB; observer disabled until restart")
	}
	switch method {
	case pluginabi.MethodResponseNormalizeBefore:
		if canObserve {
			var req pluginapi.ResponseTransformRequest
			if json.Unmarshal(raw, &req) == nil {
				runtime.ObserveRawResponse(req)
			} else {
				runtime.DisableResponseObserver("invalid response observation envelope; observer disabled until restart")
			}
		}
		return okEnvelope(pluginapi.PayloadResponse{})
	case pluginabi.MethodResponseInterceptAfter:
		if canObserve {
			var req pluginapi.ResponseInterceptRequest
			if json.Unmarshal(raw, &req) == nil {
				runtime.ObserveResponse(req)
			} else {
				runtime.DisableResponseObserver("invalid response observation envelope; observer disabled until restart")
			}
		}
		return okEnvelope(pluginapi.ResponseInterceptResponse{})
	case pluginabi.MethodResponseInterceptStreamChunk:
		if canObserve {
			var req pluginapi.StreamChunkInterceptRequest
			if json.Unmarshal(raw, &req) == nil {
				runtime.ObserveStreamResponse(req)
			} else {
				runtime.DisableResponseObserver("invalid response observation envelope; observer disabled until restart")
			}
		}
		return okEnvelope(pluginapi.StreamChunkInterceptResponse{})
	default:
		return okEnvelope(map[string]any{})
	}
}
