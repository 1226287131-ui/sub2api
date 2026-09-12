package service

import (
	"context"

	"github.com/tidwall/sjson"
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
	paths := []string{
		"usage.cache_creation",
		"usage.cache_creation_input_tokens",
		"usage.cache_creation_5m_tokens",
		"usage.cache_creation_1h_tokens",
		// OpenAI-compatible usage shapes use cache_write/cache_creation aliases,
		// often nested below input_tokens_details or prompt_tokens_details.
		"usage.cache_write_tokens",
		"usage.cache_write_input_tokens",
		"usage.cache_creation_tokens",
		"usage.input_tokens_details.cache_write_tokens",
		"usage.input_tokens_details.cache_creation_tokens",
		"usage.prompt_tokens_details.cache_write_tokens",
		"usage.prompt_tokens_details.cache_creation_tokens",
		"response.usage.cache_creation",
		"response.usage.cache_creation_input_tokens",
		"response.usage.cache_creation_5m_tokens",
		"response.usage.cache_creation_1h_tokens",
		"response.usage.cache_write_tokens",
		"response.usage.cache_write_input_tokens",
		"response.usage.cache_creation_tokens",
		"response.usage.input_tokens_details.cache_write_tokens",
		"response.usage.input_tokens_details.cache_creation_tokens",
		"response.usage.prompt_tokens_details.cache_write_tokens",
		"response.usage.prompt_tokens_details.cache_creation_tokens",
		"message.usage.cache_creation",
		"message.usage.cache_creation_input_tokens",
		"message.usage.cache_creation_5m_tokens",
		"message.usage.cache_creation_1h_tokens",
		"message.usage.cache_write_tokens",
		"message.usage.cache_write_input_tokens",
		"message.usage.cache_creation_tokens",
		"message.usage.input_tokens_details.cache_write_tokens",
		"message.usage.input_tokens_details.cache_creation_tokens",
		"message.usage.prompt_tokens_details.cache_write_tokens",
		"message.usage.prompt_tokens_details.cache_creation_tokens",
	}
	result := body
	for _, path := range paths {
		updated, err := sjson.DeleteBytes(result, path)
		if err != nil {
			return body
		}
		result = updated
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
