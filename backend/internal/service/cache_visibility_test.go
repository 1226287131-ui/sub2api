package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelHideCacheCreationOverride(t *testing.T) {
	trueValue := true
	falseValue := false

	channel := &Channel{FeaturesConfig: map[string]any{
		featureKeyHideCacheCreation: map[string]any{
			PlatformAnthropic: true,
			PlatformOpenAI:    false,
		},
	}}
	require.Equal(t, &trueValue, channel.HideCacheCreationOverride(PlatformAnthropic))
	require.Equal(t, &falseValue, channel.HideCacheCreationOverride(PlatformOpenAI))
	require.Nil(t, channel.HideCacheCreationOverride(PlatformGemini))
}

func TestHideCacheCreationContext(t *testing.T) {
	require.False(t, hideCacheCreationEnabled(context.Background()))
	require.True(t, hideCacheCreationEnabled(withHideCacheCreation(context.Background())))
}

func TestSanitizeCacheCreationJSONPreservesRawPayload(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":10,"cache_creation_input_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":7},"input_tokens_details":{"cache_write_tokens":5},"output_tokens":2},"response":{"usage":{"prompt_tokens_details":{"cache_creation_tokens":4}}},"message":{"usage":{"cache_creation_input_tokens":3}}}`)
	got := string(sanitizeCacheCreationJSON(body))
	require.NotContains(t, got, "cache_creation")
	require.NotContains(t, got, "cache_write_tokens")
	require.Contains(t, got, "input_tokens")
	require.Contains(t, got, "output_tokens")
	require.Contains(t, string(body), "cache_creation_input_tokens")
}

func TestCacheCreationResponseNormalization(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"newapi_alias_inclusive", `{"usage":{"prompt_tokens":5447,"completion_tokens":8,"total_tokens":5455,"prompt_tokens_details":{"cached_tokens":5185,"cached_creation_tokens":259}}}`, `{"usage":{"prompt_tokens":5447,"completion_tokens":8,"total_tokens":5455,"prompt_tokens_details":{"cached_tokens":5185}}}`},
		{"responses_inclusive", `{"type":"response.completed","response":{"usage":{"input_tokens":100,"output_tokens":3,"total_tokens":103,"cache_creation_input_tokens":20,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":20,"cached_creation_input_tokens":20}}}}`, `{"type":"response.completed","response":{"usage":{"input_tokens":100,"output_tokens":3,"total_tokens":103,"input_tokens_details":{"cached_tokens":40}}}}`},
		{"anthropic_exclusive", `{"type":"message","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":40,"cache_creation_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":5,"ephemeral_1h_input_tokens":15}}}`, `{"type":"message","usage":{"input_tokens":30,"output_tokens":2,"cache_read_input_tokens":40}}`},
		{"anthropic_split_only", `{"type":"message_delta","usage":{"input_tokens":10,"output_tokens":2,"claude_cache_creation_5_m_tokens":5,"claude_cache_creation_1_h_tokens":15}}`, `{"type":"message_delta","usage":{"input_tokens":30,"output_tokens":2}}`},
		{"anthropic_start", `{"type":"message_start","message":{"type":"message","usage":{"input_tokens":3,"cache_creation_input_tokens":7}}}`, `{"type":"message_start","message":{"type":"message","usage":{"input_tokens":10}}}`},
		{"keep_model_content", `{"usage":{"prompt_tokens":5,"cache_write_tokens":2},"output":[{"usage":{"cache_creation_input_tokens":99},"cache_write_tokens":88}],"metadata":{"cache_creation":123}}`, `{"usage":{"prompt_tokens":5},"output":[{"usage":{"cache_creation_input_tokens":99},"cache_write_tokens":88}],"metadata":{"cache_creation":123}}`},
		{"large_integer_untouched", `{"id":9007199254740993,"usage":{"prompt_tokens":100,"cache_write_tokens":2}}`, `{"id":9007199254740993,"usage":{"prompt_tokens":100}}`},
		{"api_data_envelope", `{"data":{"response":{"usage":{"input_tokens":12,"output_tokens":3,"input_tokens_details":{"cached_tokens":4,"cached_creation_tokens":6}}}}}`, `{"data":{"response":{"usage":{"input_tokens":12,"output_tokens":3,"input_tokens_details":{"cached_tokens":4}}}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeCacheCreationJSON([]byte(tt.body))
			assert.JSONEq(t, tt.want, string(got))
			assert.Equal(t, got, sanitizeCacheCreationJSON(got), "applying the policy twice must not add tokens twice")
		})
	}
}

func TestCacheCreationResponseRejectsMalformedTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"usage":{"cache_write_tokens":3}} junk`, `{"usage":{"cache_write_tokens":3}} {}`, `[DONE]`} {
		assert.Equal(t, body, string(sanitizeCacheCreationJSON([]byte(body))))
	}
}

func TestSanitizeCacheCreationEvent(t *testing.T) {
	event := map[string]any{"message": map[string]any{"usage": map[string]any{
		"cache_creation_input_tokens": float64(8),
		"cache_write_tokens":          float64(4),
		"input_tokens":                float64(2),
	}}}
	require.True(t, sanitizeCacheCreationEvent(event))
	require.NotContains(t, event["message"].(map[string]any)["usage"], "cache_creation_input_tokens")
	require.Contains(t, event["message"].(map[string]any)["usage"], "input_tokens")
}
