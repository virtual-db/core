package payloads

import (
	"encoding/json"
	"fmt"
)

// Decode coerces v to type T.
//
// It accepts two input shapes:
//
//  1. A value already of type T — returned as-is, zero allocation.
//  2. A map[string]any produced by a JSON round-trip across a plugin boundary —
//     re-encoded to JSON and decoded into T.
//
// # Why this is necessary
//
// Every pipeline payload that passes through a plugin's JSON-RPC handler loses
// its Go type information. The framework serialises the typed struct to JSON,
// the plugin receives raw bytes, and the returned bytes are deserialised as
// any → map[string]any. Any framework handler that runs after a plugin on the
// same point — or on any later point in the same pipeline — will therefore
// receive map[string]any instead of the expected typed payload struct.
//
// All framework handlers must call Decode instead of a bare type assertion so
// that they continue to work correctly regardless of whether a plugin is
// registered on any earlier point in the same pipeline.
func Decode[T any](v any) (T, error) {
	// Fast path: the value is already the right type. This is the common case
	// when no plugin has touched the payload on this pipeline run.
	if t, ok := v.(T); ok {
		return t, nil
	}

	// Slow path: a plugin round-trip converted the payload to map[string]any.
	// Re-encode to JSON and decode back into T. json.Unmarshal performs
	// case-insensitive field matching and handles the float64 → integer
	// conversion that results from JSON number decoding into any.
	if m, ok := v.(map[string]any); ok {
		var out T
		data, err := json.Marshal(m)
		if err != nil {
			return out, fmt.Errorf("payloads.Decode: marshal map: %w", err)
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return out, fmt.Errorf("payloads.Decode: unmarshal into %T: %w", out, err)
		}
		return out, nil
	}

	var zero T
	return zero, fmt.Errorf("payloads.Decode: cannot decode %T into %T", v, zero)
}
