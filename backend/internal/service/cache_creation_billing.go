package service

import (
	"context"
	"log/slog"
	"math"
)

// cacheCreationAsInputForBilling prefers the forwarding-time policy snapshot so
// a channel edit while the request is in flight cannot change its billing mode.
// Older/direct callers without a snapshot resolve the same group/platform rule.
func cacheCreationAsInputForBilling(ctx context.Context, snapshot *bool, channels *ChannelService, apiKey *APIKey, account *Account) bool {
	if snapshot != nil {
		return *snapshot
	}
	if channels == nil || apiKey == nil || apiKey.GroupID == nil || account == nil {
		return false
	}
	channel, err := channels.GetChannelForGroup(ctx, *apiKey.GroupID)
	if err != nil {
		slog.Warn("billing.cache_creation_policy_unavailable", "group_id", *apiKey.GroupID, "account_id", account.ID, "error", err)
		return false
	}
	override := channel.HideCacheCreationOverride(account.Platform)
	return override != nil && *override
}

// mergeClaudeCacheCreationIntoInput operates on a usage value rather than the
// upstream result, making retrying a billing operation safe from double merging.
// Anthropic input excludes cache creation; aggregate and TTL details describe
// the same tokens, so the larger total is used once, never their sum.
func mergeClaudeCacheCreationIntoInput(usage ClaudeUsage) ClaudeUsage {
	creation5m := max(usage.CacheCreation5mTokens, 0)
	creation1h := max(usage.CacheCreation1hTokens, 0)
	creation := max(usage.CacheCreationInputTokens, 0)
	input := max(usage.InputTokens, 0)
	clamped := false
	// usage_logs.input_tokens is an SQL INT. Bound the merge before integer
	// addition, so an invalid upstream count cannot overflow or prevent the row
	// from being persisted solely because this policy combined its token buckets.
	if creation5m > math.MaxInt32 || creation1h > math.MaxInt32-creation5m {
		creation = math.MaxInt32
		clamped = true
	} else {
		creation = max(creation, creation5m+creation1h)
	}
	if creation > math.MaxInt32 || input > math.MaxInt32-creation {
		usage.InputTokens = math.MaxInt32
		clamped = true
	} else {
		usage.InputTokens = input + creation
	}
	if clamped {
		slog.Warn("billing.cache_creation_input_saturated", "input_tokens", input, "cache_creation_tokens", usage.CacheCreationInputTokens,
			"cache_creation_5m_tokens", creation5m, "cache_creation_1h_tokens", creation1h, "normalized_input_tokens", usage.InputTokens)
	}
	usage.CacheCreationInputTokens = 0
	usage.CacheCreation5mTokens = 0
	usage.CacheCreation1hTokens = 0
	return usage
}
