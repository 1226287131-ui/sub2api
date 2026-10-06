package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
)

// cacheCreationVisibilityContextKey captures the forwarding-time policy.
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

// sanitizeCacheCreationJSON folds creation into regular input on a serialized
// copy. Raw usage is observed first; settlement normalizes its own value copy.
func sanitizeCacheCreationJSON(body []byte) []byte {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return body
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
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

// Only protocol envelopes are traversed. Model output and tool payloads may
// contain identically named fields and must never be rewritten.
func sanitizeCacheCreationEvent(value any) bool {
	node, ok := value.(map[string]any)
	if !ok {
		return false
	}
	changed := false
	if usage, ok := node["usage"].(map[string]any); ok {
		typ, _ := node["type"].(string)
		anthropic := typ == "message" || typ == "message_start" || typ == "message_delta"
		changed = foldCacheCreationUsage(usage, anthropic)
	}
	for _, key := range []string{"response", "message", "data"} {
		if sanitizeCacheCreationEvent(node[key]) {
			changed = true
		}
	}
	return changed
}

var cacheCreationUsageKeys = []string{
	"cache_creation_tokens", "cache_creation_input_tokens",
	"cached_creation_tokens", "cached_creation_input_tokens",
	"cache_write_tokens", "cache_write_input_tokens",
}

// Counts are bounded before addition to match the SQL INT usage columns.
func cacheUsageTokenCount(value any) int {
	const limit = int64(1<<31 - 1)
	var n int64
	switch v := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return 0
		}
		n = parsed
	case float64:
		if v >= float64(limit) {
			return int(limit)
		}
		if !(v >= 0) {
			return 0
		}
		n = int64(v)
	case int:
		n = int64(v)
	case int64:
		n = v
	default:
		return 0
	}
	if n < 0 {
		return 0
	}
	if n > limit {
		return int(limit)
	}
	return int(n)
}

func foldCacheCreationUsage(usage map[string]any, anthropic bool) bool {
	creation := ClaudeUsage{InputTokens: cacheUsageTokenCount(usage["input_tokens"])}
	changed := false
	for _, key := range cacheCreationUsageKeys {
		if value, ok := usage[key]; ok {
			creation.CacheCreationInputTokens = max(creation.CacheCreationInputTokens, cacheUsageTokenCount(value))
			delete(usage, key)
			changed = true
		}
	}
	for _, key := range []string{"cache_creation_5m_tokens", "cache_creation_tokens_5m", "claude_cache_creation_5_m_tokens"} {
		if value, ok := usage[key]; ok {
			creation.CacheCreation5mTokens = max(creation.CacheCreation5mTokens, cacheUsageTokenCount(value))
			delete(usage, key)
			changed = true
		}
	}
	for _, key := range []string{"cache_creation_1h_tokens", "cache_creation_tokens_1h", "claude_cache_creation_1_h_tokens"} {
		if value, ok := usage[key]; ok {
			creation.CacheCreation1hTokens = max(creation.CacheCreation1hTokens, cacheUsageTokenCount(value))
			delete(usage, key)
			changed = true
		}
	}
	if detail, ok := usage["cache_creation"].(map[string]any); ok {
		creation.CacheCreation5mTokens = max(creation.CacheCreation5mTokens, cacheUsageTokenCount(detail["ephemeral_5m_input_tokens"]))
		creation.CacheCreation1hTokens = max(creation.CacheCreation1hTokens, cacheUsageTokenCount(detail["ephemeral_1h_input_tokens"]))
	}
	if _, ok := usage["cache_creation"]; ok {
		delete(usage, "cache_creation")
		changed = true
	}
	for _, key := range []string{"input_tokens_details", "prompt_tokens_details"} {
		if detail, ok := usage[key].(map[string]any); ok && foldCacheCreationUsage(detail, false) {
			changed = true
		}
	}
	if anthropic && (creation.CacheCreationInputTokens > 0 || creation.CacheCreation5mTokens > 0 || creation.CacheCreation1hTokens > 0) {
		// Anthropic input excludes creation. OpenAI total input already includes
		// it, so only its breakdown is removed (never add it a second time).
		usage["input_tokens"] = mergeClaudeCacheCreationIntoInput(creation).InputTokens
	}
	return changed
}
