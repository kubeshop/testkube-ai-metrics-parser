// Package jsonutil provides small helpers for traversing decoded JSON values.
package jsonutil

import "encoding/json"

// WalkMaps invokes fn for every JSON object found anywhere in v (depth-first),
// including nested objects inside arrays. It is used by parsers that need to
// locate metric records regardless of where they sit in a framework's output,
// which keeps them resilient to schema and field-alias drift.
func WalkMaps(v any, fn func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, val := range t {
			WalkMaps(val, fn)
		}
	case []any:
		for _, val := range t {
			WalkMaps(val, fn)
		}
	}
}

// Float coerces a decoded JSON value to float64, reporting whether it was
// numeric. Booleans and strings are intentionally rejected.
func Float(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// String returns the value as a string when it is a JSON string.
func String(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// Bool returns the value as a bool when it is a JSON boolean.
func Bool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}
