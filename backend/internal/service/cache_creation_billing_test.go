//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheCreationAsInputForBillingPolicy(t *testing.T) {
	cache := newEmptyChannelCache()
	cache.loadedAt = time.Now()
	cache.channelByGroupID[17] = &Channel{
		ID: 12, Status: StatusActive,
		FeaturesConfig: map[string]any{featureKeyHideCacheCreation: map[string]any{
			PlatformOpenAI: true, PlatformAnthropic: false,
		}},
	}
	channels := &ChannelService{}
	channels.cache.Store(cache)
	apiKey := &APIKey{GroupID: i64p(17), Group: &Group{Platform: PlatformComposite}}
	account := &Account{ID: 8, Platform: PlatformOpenAI}
	on, off := true, false

	assert.True(t, cacheCreationAsInputForBilling(context.Background(), nil, channels, apiKey, account), "effective account platform, not composite group platform")
	assert.False(t, cacheCreationAsInputForBilling(context.Background(), &off, channels, apiKey, account), "an in-flight off snapshot wins over a subsequently enabled channel")
	assert.True(t, cacheCreationAsInputForBilling(context.Background(), &on, nil, nil, nil), "an in-flight on snapshot survives worker context loss")
	assert.False(t, cacheCreationAsInputForBilling(context.Background(), nil, channels, apiKey, &Account{Platform: PlatformAnthropic}))
	assert.False(t, cacheCreationAsInputForBilling(context.Background(), nil, channels, &APIKey{GroupID: i64p(99)}, account), "unassociated groups are unchanged")
	assert.False(t, cacheCreationAsInputForBilling(context.Background(), nil, nil, apiKey, account))
}

func TestOpenAIRecordUsageCacheCreationAsInput(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			svc.usageBillingRepo = billingRepo
			svc.billingService = NewBillingService(svc.cfg, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
				"gpt-5.6-sol": {InputCostPerToken: 5e-6, OutputCostPerToken: 30e-6, CacheReadInputTokenCost: 0.5e-6},
			}})
			originalUsage := OpenAIUsage{InputTokens: 1000, OutputTokens: 50, CacheCreationInputTokens: 200, CacheReadInputTokens: 100}
			result := &OpenAIForwardResult{
				RequestID: "openai-cache-merge-" + name, Model: "gpt-5.6-sol", Usage: originalUsage,
				CacheCreationAsInput: &enabled,
			}
			input := &OpenAIRecordUsageInput{Result: result, APIKey: &APIKey{ID: 2}, User: &User{ID: 3}, Account: &Account{ID: 4, Platform: PlatformOpenAI}}
			err := svc.RecordUsage(context.Background(), input)
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.NotNil(t, billingRepo.lastCmd)
			wantInput, wantCreation := 700, 200
			if enabled {
				wantInput, wantCreation = 900, 0
			}
			log := usageRepo.lastLog
			assert.Equal(t, wantInput, log.InputTokens)
			assert.Equal(t, wantCreation, log.CacheCreationTokens)
			assert.Equal(t, 100, log.CacheReadTokens)
			assert.Equal(t, 1050, log.TotalTokens(), "OpenAI input already includes cache writes: never add them twice")
			assert.InDelta(t, float64(wantInput)*5e-6, log.InputCost, 1e-12)
			assert.InDelta(t, float64(wantCreation)*6.25e-6, log.CacheCreationCost, 1e-12)
			wantTotal := float64(wantInput)*5e-6 + float64(wantCreation)*6.25e-6 + 100*0.5e-6 + 50*30e-6
			assert.InDelta(t, wantTotal, log.TotalCost, 1e-12)
			assert.InDelta(t, wantTotal*1.1, log.ActualCost, 1e-12)
			assert.Equal(t, log.InputTokens, billingRepo.lastCmd.InputTokens)
			assert.Equal(t, log.CacheCreationTokens, billingRepo.lastCmd.CacheCreationTokens)
			assert.InDelta(t, log.ActualCost, billingRepo.lastCmd.BalanceCost, 1e-12)
			assert.Equal(t, originalUsage, result.Usage, "retain the unmodified upstream snapshot")

			// The repository owns deduplication; a retried worker must submit the
			// same token buckets and charge under the same idempotency key.
			firstCommand := *billingRepo.lastCmd
			billingRepo.result = &UsageBillingApplyResult{Applied: false}
			require.NoError(t, svc.RecordUsage(context.Background(), input))
			assert.Equal(t, firstCommand, *billingRepo.lastCmd)
			assert.Equal(t, originalUsage, result.Usage)
		})
	}
}

func TestGatewayRecordUsageCacheCreationAsInput(t *testing.T) {
	for _, tc := range []struct {
		name         string
		enabled      bool
		forceCache   bool
		wantInput    int
		wantRead     int
		wantCreation int
	}{
		{name: "disabled", wantInput: 700, wantRead: 100, wantCreation: 400},
		{name: "enabled", enabled: true, wantInput: 1100, wantRead: 100},
		{name: "creation_remains_input_during_sticky_switch", enabled: true, forceCache: true, wantInput: 400, wantRead: 800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
			pricing := svc.billingService.fallbackPrices["claude-sonnet-4"]
			require.NotNil(t, pricing)
			pricing.SupportsCacheBreakdown = true
			pricing.CacheCreation5mPrice = 3.75e-6
			pricing.CacheCreation1hPrice = 6e-6
			originalUsage := ClaudeUsage{
				InputTokens: 700, OutputTokens: 50, CacheCreationInputTokens: 400,
				CacheCreation5mTokens: 250, CacheCreation1hTokens: 150, CacheReadInputTokens: 100,
			}
			result := &ForwardResult{RequestID: "anthropic-cache-merge-" + tc.name, Model: "claude-sonnet-4", Usage: originalUsage, CacheCreationAsInput: &tc.enabled}
			input := &RecordUsageInput{
				Result: result, APIKey: &APIKey{ID: 2}, User: &User{ID: 3}, Account: &Account{ID: 4, Platform: PlatformAnthropic},
				ForceCacheBilling: tc.forceCache,
			}
			require.NoError(t, svc.RecordUsage(context.Background(), input))
			require.NotNil(t, usageRepo.lastLog)
			require.NotNil(t, billingRepo.lastCmd)
			log := usageRepo.lastLog
			assert.Equal(t, tc.wantInput, log.InputTokens)
			assert.Equal(t, tc.wantRead, log.CacheReadTokens)
			assert.Equal(t, tc.wantCreation, log.CacheCreationTokens)
			assert.Equal(t, 1250, log.TotalTokens(), "Anthropic cache aggregate and TTL details overlap")
			wantCreationCost := 250*3.75e-6 + 150*6e-6
			if tc.enabled {
				wantCreationCost = 0
				assert.Zero(t, log.CacheCreation5mTokens)
				assert.Zero(t, log.CacheCreation1hTokens)
			} else {
				assert.Equal(t, 250, log.CacheCreation5mTokens)
				assert.Equal(t, 150, log.CacheCreation1hTokens)
			}
			wantTotal := float64(tc.wantInput)*3e-6 + float64(tc.wantRead)*0.3e-6 + 50*15e-6 + wantCreationCost
			assert.InDelta(t, float64(tc.wantInput)*3e-6, log.InputCost, 1e-12)
			assert.InDelta(t, wantCreationCost, log.CacheCreationCost, 1e-12)
			assert.InDelta(t, wantTotal, log.TotalCost, 1e-12)
			assert.InDelta(t, wantTotal*1.1, log.ActualCost, 1e-12)
			assert.Equal(t, log.InputTokens, billingRepo.lastCmd.InputTokens)
			assert.Equal(t, log.CacheCreationTokens, billingRepo.lastCmd.CacheCreationTokens)
			assert.InDelta(t, log.ActualCost, billingRepo.lastCmd.BalanceCost, 1e-12)
			assert.Equal(t, originalUsage, result.Usage)
			firstCommand := *billingRepo.lastCmd
			billingRepo.result = &UsageBillingApplyResult{Applied: false}
			require.NoError(t, svc.RecordUsage(context.Background(), input))
			assert.Equal(t, firstCommand, *billingRepo.lastCmd)
			assert.Equal(t, originalUsage, result.Usage)
		})
	}
}

func TestMergeClaudeCacheCreationIntoInputBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage ClaudeUsage
		want  int
	}{
		{name: "aggregate_only", usage: ClaudeUsage{InputTokens: 10, CacheCreationInputTokens: 30}, want: 40},
		{name: "ttl_only", usage: ClaudeUsage{InputTokens: 10, CacheCreation5mTokens: 20, CacheCreation1hTokens: 30}, want: 60},
		{name: "aggregate_larger", usage: ClaudeUsage{InputTokens: 10, CacheCreationInputTokens: 60, CacheCreation5mTokens: 20, CacheCreation1hTokens: 30}, want: 70},
		{name: "ttl_larger", usage: ClaudeUsage{InputTokens: 10, CacheCreationInputTokens: 40, CacheCreation5mTokens: 20, CacheCreation1hTokens: 30}, want: 60},
		{name: "negative_not_a_credit", usage: ClaudeUsage{InputTokens: -10, CacheCreationInputTokens: -20, CacheCreation5mTokens: -50, CacheCreation1hTokens: 30}, want: 30},
		{name: "input_add_overflow", usage: ClaudeUsage{InputTokens: math.MaxInt, CacheCreationInputTokens: 10}, want: math.MaxInt32},
		{name: "ttl_add_overflow", usage: ClaudeUsage{InputTokens: 10, CacheCreation5mTokens: math.MaxInt, CacheCreation1hTokens: math.MaxInt}, want: math.MaxInt32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeClaudeCacheCreationIntoInput(tc.usage)
			assert.Equal(t, tc.want, got.InputTokens)
			assert.Zero(t, got.CacheCreationInputTokens)
			assert.Zero(t, got.CacheCreation5mTokens)
			assert.Zero(t, got.CacheCreation1hTokens)
			assert.Equal(t, tc.usage.CacheReadInputTokens, got.CacheReadInputTokens)
			assert.Equal(t, tc.usage.OutputTokens, got.OutputTokens)
			assert.Equal(t, got, mergeClaudeCacheCreationIntoInput(got), "normalization is idempotent")
		})
	}
}
