package service

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testAccountProxy(id int64, status string, expiresAt *time.Time) *Proxy {
	return &Proxy{
		ID:        id,
		Name:      "proxy",
		Protocol:  "http",
		Host:      "127.0.0.1",
		Port:      8000 + int(id),
		Status:    status,
		ExpiresAt: expiresAt,
	}
}

func TestAccountProxyPoolIDsNormalizesJSONValues(t *testing.T) {
	extra := map[string]any{
		AccountProxyPoolIDsExtraKey: []any{
			float64(2), json.Number("3"), int64(2), int(4), float64(-1), float64(6.5), "5",
		},
	}
	require.Equal(t, []int64{2, 3, 4}, AccountProxyPoolIDs(extra))
}

func TestAccountNextProxyRoundRobin(t *testing.T) {
	account := &Account{
		ID:        91001,
		Proxy:     testAccountProxy(1, StatusActive, nil),
		ProxyPool: []*Proxy{testAccountProxy(2, StatusActive, nil), testAccountProxy(3, StatusActive, nil)},
		Extra: map[string]any{
			AccountProxyPoolIDsExtraKey: []int64{2, 3},
		},
	}
	accountProxyPools.Delete(account.ID)
	accountProxyPoolCounters.Delete(account.ID)
	RegisterAccountProxyPool(account)
	t.Cleanup(func() {
		accountProxyPools.Delete(account.ID)
		accountProxyPoolCounters.Delete(account.ID)
	})

	got := make([]int64, 0, 4)
	for i := 0; i < 4; i++ {
		got = append(got, account.NextProxy().ID)
	}
	require.Equal(t, []int64{1, 2, 3, 1}, got)
}

func TestAccountNextProxySkipsUnavailablePoolMembers(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute)
	account := &Account{
		ID:    91002,
		Proxy: testAccountProxy(1, StatusActive, nil),
		ProxyPool: []*Proxy{
			testAccountProxy(2, StatusDisabled, nil),
			testAccountProxy(3, StatusActive, &expiredAt),
			testAccountProxy(4, StatusActive, nil),
		},
		Extra: map[string]any{
			AccountProxyPoolIDsExtraKey: []int64{2, 3, 4, 99},
		},
	}
	accountProxyPools.Delete(account.ID)
	accountProxyPoolCounters.Delete(account.ID)
	RegisterAccountProxyPool(account)
	t.Cleanup(func() {
		accountProxyPools.Delete(account.ID)
		accountProxyPoolCounters.Delete(account.ID)
	})

	require.Equal(t, int64(1), account.NextProxy().ID)
	require.Equal(t, int64(4), account.NextProxy().ID)
	require.Equal(t, int64(1), account.NextProxy().ID)
}

func TestAccountNextProxyLegacyPrimaryOnly(t *testing.T) {
	primary := testAccountProxy(1, StatusActive, nil)
	account := &Account{ID: 91003, Proxy: primary}
	require.Same(t, primary, account.NextProxy())
	require.Equal(t, primary.URL(), account.NextProxyURL())
}

func TestProxyLanesPreserveHydratedLegacyAccount(t *testing.T) {
	for _, concurrency := range []int{0, 7} {
		t.Run(strconv.Itoa(concurrency), func(t *testing.T) {
			primary := testAccountProxy(1, StatusActive, nil)
			account := &Account{
				ID: 91900, ProxyID: &primary.ID, Proxy: primary,
				Concurrency: concurrency, ProxyPoolHydrated: true,
			}
			t.Cleanup(func() { accountProxyPools.Delete(account.ID) })
			account.Extra = NormalizeAccountProxyLaneExtra(nil, account.ProxyID, concurrency)
			require.NotContains(t, account.Extra, AccountProxyLaneConfigsExtraKey)
			RegisterAccountProxyPool(account)
			require.Equal(t, concurrency, account.EffectiveConcurrency())
			require.Empty(t, account.ProxyLaneConfigs(), "account DTOs must not activate lanes on a later edit")
			require.Equal(t, primary.URL(), account.NextProxyLaneURL())
			for range 2 {
				url, proxyID, limit, _, release, err := AcquireAccountProxyLaneForURL(account.ID, account.NextProxyLaneURL())
				require.NoError(t, err)
				require.Equal(t, primary.URL(), url)
				require.Zero(t, proxyID)
				require.Zero(t, limit)
				t.Cleanup(func() { release(true) })
			}
		})
	}
}

func TestNormalizeAccountProxyPoolExtraDropsPrimaryAndDuplicates(t *testing.T) {
	primaryID := int64(2)
	extra := NormalizeAccountProxyPoolExtra(map[string]any{
		AccountProxyPoolIDsExtraKey: []any{float64(2), float64(3), float64(3), float64(4)},
	}, &primaryID)
	require.Equal(t, []int64{3, 4}, AccountProxyPoolIDs(extra))
}

func TestProxyLanesRespectCapacityAndRelease(t *testing.T) {
	account := &Account{
		ID:          91004,
		Proxy:       testAccountProxy(1, StatusActive, nil),
		ProxyPool:   []*Proxy{testAccountProxy(2, StatusActive, nil)},
		Concurrency: 4,
		Extra: map[string]any{
			AccountProxyPoolIDsExtraKey: []int64{2},
			AccountProxyLaneConfigsExtraKey: []ProxyLaneConfig{
				{ProxyID: 1, Enabled: true, MaxConcurrency: 1, Weight: 1},
				{ProxyID: 2, Enabled: true, MaxConcurrency: 2, Weight: 2},
			},
			AccountProxyLaneStrategyExtraKey: ProxyLaneStrategyLeastConnections,
		},
	}
	accountProxyPools.Delete(account.ID)
	RegisterAccountProxyPool(account)
	t.Cleanup(func() { accountProxyPools.Delete(account.ID) })

	firstURL, firstID, _, firstTimeout, firstRelease, err := AcquireAccountProxyLaneForURL(account.ID, account.NextProxyLaneURL())
	require.NoError(t, err)
	require.NotEmpty(t, firstURL)
	require.Zero(t, firstTimeout)
	require.NotZero(t, firstID)
	secondURL, secondID, _, _, secondRelease, err := AcquireAccountProxyLaneForURL(account.ID, account.NextProxyLaneURL())
	require.NoError(t, err)
	require.NotEmpty(t, secondURL)
	require.NotEqual(t, firstID, secondID, "least-connections should move to the idle lane")
	_, thirdID, _, _, thirdRelease, err := AcquireAccountProxyLaneForURL(account.ID, account.NextProxyLaneURL())
	require.NoError(t, err)
	require.NotZero(t, thirdID)

	statuses := AccountProxyLaneStatuses(account)
	require.Len(t, statuses, 2)
	current := map[int64]int{}
	for _, status := range statuses {
		current[status.ProxyID] = status.CurrentConcurrency
	}
	require.Equal(t, 1, current[firstID])
	require.GreaterOrEqual(t, current[secondID], 1)

	firstRelease(true)
	secondRelease(true)
	thirdRelease(true)
	for _, status := range AccountProxyLaneStatuses(account) {
		require.Equal(t, 0, status.CurrentConcurrency)
	}
}

func TestProxyLaneCircuitBreaksAfterTransportFailures(t *testing.T) {
	account := &Account{
		ID: 91005, Proxy: testAccountProxy(1, StatusActive, nil), Concurrency: 2,
		Extra: map[string]any{
			AccountProxyPoolIDsExtraKey: []int64{2},
			AccountProxyLaneConfigsExtraKey: []ProxyLaneConfig{
				{ProxyID: 1, Enabled: true, MaxConcurrency: 1, ErrorCircuitThreshold: 1, CircuitCooldownSeconds: 60},
				{ProxyID: 2, Enabled: true, MaxConcurrency: 1},
			},
		},
		ProxyPool: []*Proxy{testAccountProxy(2, StatusActive, nil)},
	}
	RegisterAccountProxyPool(account)
	t.Cleanup(func() { accountProxyPools.Delete(account.ID) })

	_, id, _, _, release, err := AcquireAccountProxyLaneForURL(account.ID, markAccountProxyLaneURL(account.Proxy.URL(), account.ID, 1))
	require.NoError(t, err)
	require.Equal(t, int64(1), id)
	release(false)

	_, id, _, _, _, err = AcquireAccountProxyLaneForURL(account.ID, markAccountProxyLaneURL(account.Proxy.URL(), account.ID, 1))
	require.NoError(t, err)
	require.Equal(t, int64(2), id, "open circuit should fall back to the next healthy lane")
}
