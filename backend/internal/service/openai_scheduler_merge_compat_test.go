package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type schedulerCapacityCache struct {
	schedulerTestConcurrencyCache
	attempts []AccountWithConcurrency
}

func (c *schedulerCapacityCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	c.attempts = append(c.attempts, AccountWithConcurrency{ID: accountID, MaxConcurrency: maxConcurrency})
	return false, nil
}

func TestOpenAISelectionWaitPlanPreservesProxyLaneCapacity(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		name := "legacy"
		if advanced {
			name = "advanced"
		}
		t.Run(name, func(t *testing.T) {
			groupID := int64(9040)
			proxyID := int64(41)
			accounts := []Account{
				{
					ID: 9041, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
					Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
					ProxyID: &proxyID,
					Extra: map[string]any{
						AccountProxyPoolIDsExtraKey: []int64{42},
						AccountProxyLaneConfigsExtraKey: []ProxyLaneConfig{
							{ProxyID: 41, Enabled: true, MaxConcurrency: 2},
							{ProxyID: 42, Enabled: true, MaxConcurrency: 3},
						},
					},
				},
				{ID: 9042, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 3, GroupIDs: []int64{groupID}},
			}
			cache := &schedulerCapacityCache{}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			cfg.Gateway.Scheduling.LoadBatchEnabled = true
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
				cfg:                cfg,
				concurrencyService: NewConcurrencyService(cache),
			}
			var selection *AccountSelectionResult
			var err error
			if advanced {
				selection, _, err = newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), OpenAIAccountScheduleRequest{
					GroupID: &groupID, RequestedModel: "gpt-5.1", RequiredTransport: OpenAIUpstreamTransportAny,
				})
			} else {
				selection, err = svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.1", nil)
			}
			require.NoError(t, err)
			require.NotNil(t, selection)
			assert.False(t, selection.Acquired)
			require.NotNil(t, selection.WaitPlan)
			require.Len(t, selection.WaitPlan.Candidates, 2)
			expected := map[int64]int{9041: 5, 9042: 3}
			assert.Equal(t, expected[selection.Account.ID], selection.WaitPlan.MaxConcurrency)
			for _, candidate := range selection.WaitPlan.Candidates {
				assert.Equal(t, expected[candidate.Account.ID], candidate.MaxConcurrency)
			}
			require.NotEmpty(t, cache.attempts)
			for _, attempt := range cache.attempts {
				assert.Equal(t, expected[attempt.ID], attempt.MaxConcurrency)
			}
		})
	}
}

func TestOpenAISelectionWaitCandidatesHonorMixedTicketPolicies(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			name := "legacy"
			if advanced {
				name = "advanced"
			}
			if enabled {
				name += "/tickets_enabled"
			} else {
				name += "/tickets_disabled"
			}
			t.Run(name, func(t *testing.T) {
				groupID := int64(9050)
				model := openAICodexTicketDefaultModel
				accounts := []Account{
					{ID: 9051, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, Extra: map[string]any{
						OpenAICodexTicketMissingPolicyExtraKey: openAICodexTicketMissingPolicyPause,
					}},
					{ID: 9052, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, Extra: map[string]any{
						OpenAICodexTicketMissingPolicyExtraKey:      openAICodexTicketMissingPolicyPause,
						openAICodexTicketModelPolicyExtraKey(model): map[string]any{"enabled": false},
					}},
					{ID: 9053, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, Extra: map[string]any{
						OpenAICodexTicketMissingPolicyExtraKey: openAICodexTicketMissingPolicyAllow,
					}},
				}
				cache := &schedulerCapacityCache{}
				cfg := &config.Config{RunMode: config.RunModeStandard}
				cfg.Gateway.Scheduling.LoadBatchEnabled = true
				cfg.Gateway.OpenAICodexTicket.Enabled = enabled
				svc := &OpenAIGatewayService{
					accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
					cfg:                cfg,
					concurrencyService: NewConcurrencyService(cache),
				}
				var selection *AccountSelectionResult
				var err error
				if advanced {
					selection, _, err = newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), OpenAIAccountScheduleRequest{
						GroupID: &groupID, RequestedModel: model, RequiredTransport: OpenAIUpstreamTransportAny,
					})
				} else {
					selection, err = svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", model, nil)
				}
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.NotNil(t, selection.WaitPlan)
				ids := make([]int64, 0, len(selection.WaitPlan.Candidates))
				for _, candidate := range selection.WaitPlan.Candidates {
					ids = append(ids, candidate.Account.ID)
				}
				expected := []int64{9052, 9053}
				if !enabled {
					expected = append(expected, 9051)
				}
				assert.ElementsMatch(t, expected, ids)
				for _, attempt := range cache.attempts {
					assert.Contains(t, expected, attempt.ID, "ticket-paused accounts must not compete for slots")
				}
			})
		}
	}
}

func TestOpenAIWaitCandidatesRetainValidatedAccountAtProbeLimit(t *testing.T) {
	accounts := []Account{
		{ID: 9061, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1},
		{ID: 9062, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1},
	}
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
		cfg:         &config.Config{RunMode: config.RunModeStandard},
		schedulerSnapshot: &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
			accountsByID: map[int64]*Account{9061: &accounts[0], 9062: &accounts[1]},
		}},
	}
	scheduler := newDefaultOpenAIAccountScheduler(svc, nil).(*defaultOpenAIAccountScheduler)
	order := []openAIAccountCandidateScore{{account: &accounts[0]}, {account: &accounts[1]}}
	budget := newOpenAISelectionProbeBudget()
	budget.enableLimit()
	budget.rechecks = openAIAccountSelectionProbeLimit - 1
	selection, _, _, _, err := scheduler.finishLoadBalanceSelectionFallback(context.Background(), OpenAIAccountScheduleRequest{
		RequestedModel: "gpt-5.1", RequiredTransport: OpenAIUpstreamTransportAny,
	}, openAIAccountLoadSelectionAttempt{selectionOrder: order, candidateCount: 2}, budget, openAISelectionFilterStats{})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.WaitPlan)
	require.Len(t, selection.WaitPlan.Candidates, 1)
	assert.Equal(t, int64(9061), selection.WaitPlan.AccountID)
}

func TestOpenAISelectionCompactTicketGateUsesOutboundModel(t *testing.T) {
	for _, mode := range []struct {
		name      string
		advanced  bool
		loadBatch bool
	}{
		{name: "legacy"},
		{name: "legacy_load", loadBatch: true},
		{name: "advanced", advanced: true, loadBatch: true},
	} {
		for _, tc := range []struct {
			name              string
			compact           bool
			busy              bool
			cachedUnsupported bool
			dbUnsupported     bool
			outboundGated     bool
			wantErr           error
		}{
			{name: "acquired", compact: true},
			{name: "wait", compact: true, busy: true},
			{name: "stale_compact_probe", compact: true, cachedUnsupported: true},
			{name: "stale_compact_probe_wait", compact: true, busy: true, cachedUnsupported: true},
			{name: "unsupported", compact: true, dbUnsupported: true, wantErr: ErrNoAvailableCompactAccounts},
			{name: "unsupported_wait", compact: true, busy: true, dbUnsupported: true, wantErr: ErrNoAvailableCompactAccounts},
			{name: "ordinary_request_requires_ticket", wantErr: ErrNoAvailableAccounts},
			{name: "compact_outbound_requires_ticket", compact: true, outboundGated: true, wantErr: ErrNoAvailableAccounts},
		} {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
				groupID := int64(9070)
				account := Account{
					ID: 9071, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
					Extra: map[string]any{
						"openai_compact_supported":             !tc.dbUnsupported,
						OpenAICodexTicketMissingPolicyExtraKey: openAICodexTicketMissingPolicyPause,
					},
				}
				cached := account
				cached.Extra = map[string]any{
					"openai_compact_supported":             !tc.cachedUnsupported,
					OpenAICodexTicketMissingPolicyExtraKey: openAICodexTicketMissingPolicyPause,
				}
				cfg := &config.Config{RunMode: config.RunModeStandard}
				cfg.Gateway.Scheduling.LoadBatchEnabled = mode.loadBatch
				cfg.Gateway.OpenAICompactModel = "gpt-5.5"
				cfg.Gateway.OpenAICodexTicket = config.OpenAICodexTicketConfig{
					Enabled: true, FailClosed: true, Models: []string{"gpt-6-astra"},
				}
				model := "gpt-6-astra"
				if tc.outboundGated {
					cfg.Gateway.OpenAICompactModel = "gpt-6-astra"
					model = "gpt-5.5"
				}
				var acquiredIDs, releasedIDs []int64
				svc := &OpenAIGatewayService{
					accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
					cfg:         cfg,
					schedulerSnapshot: &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
						snapshotAccounts: []*Account{&cached},
						accountsByID:     map[int64]*Account{cached.ID: &cached},
					}},
					concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
						acquireResults: map[int64]bool{account.ID: !tc.busy},
						acquiredIDs:    &acquiredIDs,
						releasedIDs:    &releasedIDs,
					}),
				}
				if mode.advanced {
					svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
				}
				selection, _, err := svc.SelectAccountWithScheduler(
					context.Background(), &groupID, "", "", model, nil, OpenAIUpstreamTransportAny, tc.compact,
				)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
					assert.Nil(t, selection)
					if !tc.busy {
						assert.ElementsMatch(t, acquiredIDs, releasedIDs, "rejected accounts must release every acquired slot")
					}
					if tc.wantErr == ErrNoAvailableAccounts {
						assert.NotErrorIs(t, err, ErrNoAvailableCompactAccounts)
					}
					return
				}
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.NotNil(t, selection.Account)
				assert.Equal(t, account.ID, selection.Account.ID)
				assert.Equal(t, !tc.busy, selection.Acquired)
				if tc.busy {
					require.NotNil(t, selection.WaitPlan)
					assert.Equal(t, account.ID, selection.WaitPlan.AccountID)
				} else {
					require.NotNil(t, selection.ReleaseFunc)
					selection.ReleaseFunc()
				}
			})
		}
	}
}
