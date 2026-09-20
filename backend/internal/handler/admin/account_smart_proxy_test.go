package admin

import (
	"context"
	"math/rand"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPlanSmartProxyAssignmentsNoDuplicates(t *testing.T) {
	plans := planSmartProxyAssignments(
		[]int64{1, 2},
		[]smartProxyCandidate{
			{ID: 11, LatencyMs: 20},
			{ID: 12, LatencyMs: 30},
			{ID: 13, LatencyMs: 40},
		},
		SmartProxyAssignmentOptions{ProxyCount: 2, PreferLowLatency: true, WeightedByLoad: true},
		rand.New(rand.NewSource(1)),
	)
	require.Len(t, plans[1], 2)
	require.Len(t, plans[2], 2)
	require.NotEqual(t, plans[1][0], plans[1][1])
	require.NotEqual(t, plans[2][0], plans[2][1])
}

func TestPlanSmartProxyAssignmentsWeightsLatencyAndLoad(t *testing.T) {
	accountIDs := make([]int64, 300)
	for i := range accountIDs {
		accountIDs[i] = int64(i + 1)
	}
	plans := planSmartProxyAssignments(
		accountIDs,
		[]smartProxyCandidate{
			{ID: 11, LatencyMs: 20, AccountCount: 0},
			{ID: 12, LatencyMs: 800, AccountCount: 80},
		},
		SmartProxyAssignmentOptions{ProxyCount: 1, PreferLowLatency: true, WeightedByLoad: true},
		rand.New(rand.NewSource(42)),
	)
	counts := map[int64]int{}
	for _, selected := range plans {
		counts[selected[0]]++
	}
	require.Greater(t, counts[11], counts[12]*3)
}

func TestSmartAssignAccountProxiesTestsAndPersists(t *testing.T) {
	svc := newStubAdminService()
	svc.proxyCounts = []service.ProxyWithAccountCount{
		{Proxy: service.Proxy{ID: 1, Protocol: "http", Host: "127.0.0.1", Port: 8001, Status: service.StatusActive}},
		{Proxy: service.Proxy{ID: 2, Protocol: "http", Host: "127.0.0.1", Port: 8002, Status: service.StatusActive}},
		{Proxy: service.Proxy{ID: 3, Protocol: "http", Host: "127.0.0.1", Port: 8003, Status: service.StatusActive}},
	}
	h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	result, err := h.smartAssignAccountProxies(context.Background(), []int64{101, 102}, SmartProxyAssignmentOptions{
		ProxyCount: 2, TestLatency: true, PreferLowLatency: true, WeightedByLoad: true,
	}, nil)

	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.Zero(t, result.Failed)
	require.Equal(t, 3, result.TestedProxies)
	require.ElementsMatch(t, []int64{1, 2, 3}, svc.testedProxyIDs)
	require.Equal(t, 2, svc.updateAccountCalls)
	for _, item := range result.Items {
		require.True(t, item.Success)
		require.NotZero(t, item.PrimaryProxyID)
		require.Len(t, item.ProxyPoolIDs, 1)
		require.NotEqual(t, item.PrimaryProxyID, item.ProxyPoolIDs[0])
	}
}
