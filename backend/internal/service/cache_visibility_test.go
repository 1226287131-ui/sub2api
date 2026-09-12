package service

import (
	"context"
	"testing"

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

func TestSanitizeCacheCreationJSONPreservesBillingPayloadOutsideResponse(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":10,"cache_creation_input_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":7},"input_tokens_details":{"cache_write_tokens":5},"output_tokens":2},"response":{"usage":{"prompt_tokens_details":{"cache_creation_tokens":4}}},"message":{"usage":{"cache_creation_input_tokens":3}}}`)
	got := string(sanitizeCacheCreationJSON(body))
	require.NotContains(t, got, "cache_creation")
	require.NotContains(t, got, "cache_write_tokens")
	require.Contains(t, got, "input_tokens")
	require.Contains(t, got, "output_tokens")
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
