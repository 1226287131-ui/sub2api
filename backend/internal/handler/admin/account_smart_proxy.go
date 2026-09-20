package admin

import (
	"context"
	"math/rand"
	"sort"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const smartProxyTestConcurrency = 8

type SmartProxyAssignmentOptions struct {
	Enabled          bool `json:"enabled,omitempty"`
	ProxyCount       int  `json:"proxy_count"`
	TestLatency      bool `json:"test_latency"`
	PreferLowLatency bool `json:"prefer_low_latency"`
	LowLatencyLimit  int  `json:"low_latency_limit"`
	WeightedByLoad   bool `json:"weighted_by_load"`
}

type SmartProxyAssignmentRequest struct {
	AccountIDs []int64                     `json:"account_ids" binding:"required,min=1"`
	Options    SmartProxyAssignmentOptions `json:"options"`
}

type SmartProxyAssignmentItem struct {
	AccountID      int64   `json:"account_id"`
	Success        bool    `json:"success"`
	PrimaryProxyID int64   `json:"primary_proxy_id,omitempty"`
	ProxyPoolIDs   []int64 `json:"proxy_pool_ids,omitempty"`
	Error          string  `json:"error,omitempty"`
}

type SmartProxyAssignmentResult struct {
	Success          int                        `json:"success"`
	Failed           int                        `json:"failed"`
	TestedProxies    int                        `json:"tested_proxies"`
	AvailableProxies int                        `json:"available_proxies"`
	Items            []SmartProxyAssignmentItem `json:"items"`
}

type smartProxyCandidate struct {
	ID           int64
	LatencyMs    int64
	AccountCount int64
}

func normalizeSmartProxyAssignmentOptions(options SmartProxyAssignmentOptions) (SmartProxyAssignmentOptions, error) {
	if options.ProxyCount <= 0 {
		options.ProxyCount = 2
	}
	if options.ProxyCount > 32 {
		return options, infraerrors.BadRequest("SMART_PROXY_COUNT_INVALID", "proxy_count must be between 1 and 32")
	}
	if options.LowLatencyLimit < 0 {
		return options, infraerrors.BadRequest("SMART_PROXY_LIMIT_INVALID", "low_latency_limit must be >= 0")
	}
	return options, nil
}

// SmartAssignProxies tests and distributes active proxies across accounts.
// POST /api/v1/admin/accounts/smart-assign-proxies
func (h *AccountHandler) SmartAssignProxies(c *gin.Context) {
	var req SmartProxyAssignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	options, err := normalizeSmartProxyAssignmentOptions(req.Options)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.smartAssignAccountProxies(c.Request.Context(), req.AccountIDs, options, nil)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AccountHandler) smartAssignAccountProxies(
	ctx context.Context,
	accountIDs []int64,
	options SmartProxyAssignmentOptions,
	allowedProxyIDs map[int64]struct{},
) (*SmartProxyAssignmentResult, error) {
	options, err := normalizeSmartProxyAssignmentOptions(options)
	if err != nil {
		return nil, err
	}
	accountIDs = uniqueSmartProxyIDs(accountIDs)
	if len(accountIDs) == 0 {
		return &SmartProxyAssignmentResult{Items: []SmartProxyAssignmentItem{}}, nil
	}

	proxies, err := h.adminService.GetAllProxiesWithAccountCount(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	eligible := make([]service.ProxyWithAccountCount, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy.ID <= 0 || !proxy.IsActive() || proxy.IsExpired(now) {
			continue
		}
		if allowedProxyIDs != nil {
			if _, ok := allowedProxyIDs[proxy.ID]; !ok {
				continue
			}
		}
		eligible = append(eligible, proxy)
	}
	if len(eligible) == 0 {
		return nil, infraerrors.BadRequest("SMART_PROXY_EMPTY", "no active proxies are available")
	}

	candidates, tested := h.smartProxyCandidates(ctx, eligible, options.TestLatency)
	if len(candidates) < options.ProxyCount {
		return nil, infraerrors.BadRequest(
			"SMART_PROXY_INSUFFICIENT",
			"not enough healthy proxies for the requested parallel proxy count",
		)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].LatencyMs != candidates[j].LatencyMs {
			return candidates[i].LatencyMs < candidates[j].LatencyMs
		}
		if candidates[i].AccountCount != candidates[j].AccountCount {
			return candidates[i].AccountCount < candidates[j].AccountCount
		}
		return candidates[i].ID < candidates[j].ID
	})
	if options.LowLatencyLimit > 0 && options.LowLatencyLimit < len(candidates) {
		limit := options.LowLatencyLimit
		if limit < options.ProxyCount {
			limit = options.ProxyCount
		}
		candidates = candidates[:limit]
	}

	plans := planSmartProxyAssignments(
		accountIDs,
		candidates,
		options,
		rand.New(rand.NewSource(time.Now().UnixNano())),
	)
	result := &SmartProxyAssignmentResult{
		TestedProxies:    tested,
		AvailableProxies: len(candidates),
		Items:            make([]SmartProxyAssignmentItem, 0, len(accountIDs)),
	}
	for _, accountID := range accountIDs {
		selected := plans[accountID]
		item := SmartProxyAssignmentItem{AccountID: accountID}
		if len(selected) == 0 {
			item.Error = "no proxy assignment generated"
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		primaryID := selected[0]
		poolIDs := append([]int64(nil), selected[1:]...)
		if _, updateErr := h.adminService.UpdateAccount(ctx, accountID, &service.UpdateAccountInput{
			ProxyID:      &primaryID,
			ProxyPoolIDs: &poolIDs,
		}); updateErr != nil {
			item.Error = updateErr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		item.Success = true
		item.PrimaryProxyID = primaryID
		item.ProxyPoolIDs = poolIDs
		result.Success++
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (h *AccountHandler) smartProxyCandidates(
	ctx context.Context,
	proxies []service.ProxyWithAccountCount,
	testLatency bool,
) ([]smartProxyCandidate, int) {
	if !testLatency {
		out := make([]smartProxyCandidate, 0, len(proxies))
		for _, proxy := range proxies {
			latency := int64(10_000)
			if proxy.LatencyStatus == "success" && proxy.LatencyMs != nil && *proxy.LatencyMs > 0 {
				latency = *proxy.LatencyMs
			}
			out = append(out, smartProxyCandidate{ID: proxy.ID, LatencyMs: latency, AccountCount: proxy.AccountCount})
		}
		return out, 0
	}

	type testedProxy struct {
		candidate smartProxyCandidate
		ok        bool
	}
	jobs := make(chan service.ProxyWithAccountCount)
	results := make(chan testedProxy, len(proxies))
	workers := smartProxyTestConcurrency
	if len(proxies) < workers {
		workers = len(proxies)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for proxy := range jobs {
				test, testErr := h.adminService.TestProxy(ctx, proxy.ID)
				if testErr != nil || test == nil || !test.Success || test.LatencyMs <= 0 {
					results <- testedProxy{}
					continue
				}
				results <- testedProxy{ok: true, candidate: smartProxyCandidate{
					ID: proxy.ID, LatencyMs: test.LatencyMs, AccountCount: proxy.AccountCount,
				}}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, proxy := range proxies {
			select {
			case jobs <- proxy:
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	close(results)
	out := make([]smartProxyCandidate, 0, len(proxies))
	for result := range results {
		if result.ok {
			out = append(out, result.candidate)
		}
	}
	return out, len(proxies)
}

func planSmartProxyAssignments(
	accountIDs []int64,
	candidates []smartProxyCandidate,
	options SmartProxyAssignmentOptions,
	rng *rand.Rand,
) map[int64][]int64 {
	plannedLoad := make(map[int64]int64, len(candidates))
	plans := make(map[int64][]int64, len(accountIDs))
	for _, accountID := range accountIDs {
		available := append([]smartProxyCandidate(nil), candidates...)
		selected := make([]int64, 0, options.ProxyCount)
		for len(selected) < options.ProxyCount && len(available) > 0 {
			weights := make([]float64, len(available))
			total := 0.0
			for i, candidate := range available {
				weight := 1.0
				if options.PreferLowLatency {
					weight *= 1000.0 / float64(candidate.LatencyMs+50)
				}
				if options.WeightedByLoad {
					weight /= float64(1 + candidate.AccountCount + plannedLoad[candidate.ID])
				}
				weights[i] = weight
				total += weight
			}
			index := rng.Intn(len(available))
			if total > 0 {
				pick := rng.Float64() * total
				for i, weight := range weights {
					pick -= weight
					if pick <= 0 {
						index = i
						break
					}
				}
			}
			chosen := available[index]
			selected = append(selected, chosen.ID)
			plannedLoad[chosen.ID]++
			available = append(available[:index], available[index+1:]...)
		}
		plans[accountID] = selected
	}
	return plans
}

func uniqueSmartProxyIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
