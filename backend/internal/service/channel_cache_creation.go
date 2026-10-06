package service

import "strings"

const featureKeyHideCacheCreation = "hide_cache_creation"

// HideCacheCreationOverride returns the channel policy for one upstream
// platform. A nil value preserves the existing upstream cache-creation fields.
func (c *Channel) HideCacheCreationOverride(platform string) *bool {
	if c == nil || c.FeaturesConfig == nil {
		return nil
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil
	}
	policies, ok := c.FeaturesConfig[featureKeyHideCacheCreation].(map[string]any)
	if !ok {
		return nil
	}
	enabled, ok := policies[platform].(bool)
	if !ok {
		return nil
	}
	return &enabled
}
