package service

import (
	"bytes"
	"context"
	"encoding/json"
)

// cacheCreationVisibilityContextKey is intentionally private so only gateway
// response paths can opt into response sanitization.
type cacheCreationVisibilityContextKey struct{}

func withHideCacheCreation(ctx context.Context) context.Context {
	return context.WithValue(ctx, cacheCreationVisibilityContextKey{}, true)
}

func hideCacheCreationEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(cacheCreationVisibilityContextKey{}).(bool)
	return enabled
}

// sanitizeCacheCreationJSON removes cache creation fields from a client-facing
// JSON document. It deliberately operates on a copy of the serialized body;
// callers must parse/observe usage before invoking it.
func sanitizeCacheCreationJSON(body []byte) []byte {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return body
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return body
	}
	if !sanitizeCacheCreationEvent(value) {
		return body
	}
	result, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return result
}

// sanitizeCacheCreationEvent removes the same fields from decoded SSE events,
// including both top-level usage and message.usage shapes.
func sanitizeCacheCreationEvent(value any) bool {
	changed := false
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if key == "cache_creation" || key == "cache_creation_input_tokens" ||
				key == "cache_creation_5m_tokens" || key == "cache_creation_1h_tokens" ||
				key == "cache_write_tokens" || key == "cache_write_input_tokens" ||
				key == "cache_creation_tokens" {
				delete(node, key)
				changed = true
				continue
			}
			if sanitizeCacheCreationEvent(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range node {
			if sanitizeCacheCreationEvent(child) {
				changed = true
			}
		}
	}
	return changed
}
