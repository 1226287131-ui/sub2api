package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type proxyLaneRuntime struct {
	proxy  *Proxy
	config ProxyLaneConfig
	active atomic.Int64

	mu            sync.Mutex
	failures      int
	circuitOpenTo time.Time
}

func (lane *proxyLaneRuntime) snapshot() (*Proxy, ProxyLaneConfig, int, time.Time) {
	if lane == nil {
		return nil, ProxyLaneConfig{}, 0, time.Time{}
	}
	lane.mu.Lock()
	defer lane.mu.Unlock()
	return lane.proxy, lane.config, lane.failures, lane.circuitOpenTo
}

type accountProxyPoolRuntime struct {
	accountID int64
	strategy  string
	lanes     []*proxyLaneRuntime
	counter   atomic.Uint64
}

type ProxyLaneStatus struct {
	ProxyID                int64      `json:"proxy_id"`
	Name                   string     `json:"name"`
	Primary                bool       `json:"primary"`
	Enabled                bool       `json:"enabled"`
	Healthy                bool       `json:"healthy"`
	CurrentConcurrency     int        `json:"current_concurrency"`
	MaxConcurrency         int        `json:"max_concurrency"`
	Weight                 int        `json:"weight"`
	TimeoutSeconds         int        `json:"timeout_seconds"`
	ErrorCircuitThreshold  int        `json:"error_circuit_threshold"`
	CircuitCooldownSeconds int        `json:"circuit_cooldown_seconds"`
	FallbackOrder          int        `json:"fallback_order"`
	CircuitOpenUntil       *time.Time `json:"circuit_open_until,omitempty"`
	Status                 string     `json:"status"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
}

var accountProxyPools sync.Map // accountID -> *accountProxyPoolRuntime
// Kept for source compatibility with older focused tests; counters now live on
// each account pool and lane rather than in a second global map.
var accountProxyPoolCounters sync.Map

var ErrAccountProxyLaneUnavailable = errors.New("all proxy lanes are unavailable or at capacity")

const proxyLaneURLMarker = "#sub2api-proxy-lane="

type accountProxyPoolResolvedContextKey struct{}
type accountProxyPoolEligibleContextKey struct{}

// WithAccountProxyPoolEligible marks a real user-facing upstream request as
// eligible for proxy-lane scheduling. Maintenance traffic deliberately does
// not carry this marker and therefore keeps using the account's primary proxy.
func WithAccountProxyPoolEligible(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, accountProxyPoolEligibleContextKey{}, true)
}

func AccountProxyPoolEligible(ctx context.Context) bool {
	return ctx != nil && ctx.Value(accountProxyPoolEligibleContextKey{}) == true
}

func WithAccountProxyPoolResolved(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, accountProxyPoolResolvedContextKey{}, true)
}

func AccountProxyPoolResolved(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	resolved, _ := ctx.Value(accountProxyPoolResolvedContextKey{}).(bool)
	return resolved
}

func buildProxyLaneRuntime(account *Account, proxy *Proxy, old map[int64]*proxyLaneRuntime) *proxyLaneRuntime {
	if proxy == nil || proxy.ID <= 0 {
		return nil
	}
	configs := account.ProxyLaneConfigs()
	config := ProxyLaneConfig{ProxyID: proxy.ID, Enabled: true, MaxConcurrency: max(1, account.Concurrency), Weight: 1}
	for _, candidate := range configs {
		if candidate.ProxyID == proxy.ID {
			config = candidate
			break
		}
	}
	if previous := old[proxy.ID]; previous != nil {
		previous.mu.Lock()
		previous.proxy = proxy
		previous.config = config
		previous.mu.Unlock()
		return previous
	}
	return &proxyLaneRuntime{proxy: proxy, config: config}
}

// RegisterAccountProxyPool publishes the hydrated proxy lanes for central
// HTTP-upstream selection. Runtime counters and circuit state survive snapshot
// refreshes for lane IDs that remain attached.
func RegisterAccountProxyPool(account *Account) {
	if account == nil || account.ID <= 0 {
		return
	}
	if !accountProxyLanesConfigured(account.Extra) {
		accountProxyPools.Delete(account.ID)
		return
	}
	old := map[int64]*proxyLaneRuntime{}
	oldCounter := uint64(0)
	if raw, ok := accountProxyPools.Load(account.ID); ok {
		if pool, valid := raw.(*accountProxyPoolRuntime); valid && pool != nil {
			oldCounter = pool.counter.Load()
			for _, lane := range pool.lanes {
				proxy, _, _, _ := lane.snapshot()
				if proxy != nil {
					old[proxy.ID] = lane
				}
			}
		}
	}
	proxies := make([]*Proxy, 0, len(account.ProxyPool)+1)
	proxies = append(proxies, account.Proxy)
	proxies = append(proxies, account.ProxyPool...)
	seen := map[int64]struct{}{}
	lanes := make([]*proxyLaneRuntime, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil || proxy.ID <= 0 {
			continue
		}
		if _, exists := seen[proxy.ID]; exists {
			continue
		}
		seen[proxy.ID] = struct{}{}
		if lane := buildProxyLaneRuntime(account, proxy, old); lane != nil {
			lanes = append(lanes, lane)
		}
	}
	sort.SliceStable(lanes, func(i, j int) bool {
		_, left, _, _ := lanes[i].snapshot()
		_, right, _, _ := lanes[j].snapshot()
		return left.FallbackOrder < right.FallbackOrder
	})
	pool := &accountProxyPoolRuntime{
		accountID: account.ID,
		strategy:  ProxyLaneStrategy(account.Extra),
		lanes:     lanes,
	}
	pool.counter.Store(oldCounter)
	accountProxyPools.Store(account.ID, pool)
}

func laneCircuitOpen(lane *proxyLaneRuntime, now time.Time) (bool, time.Time) {
	lane.mu.Lock()
	defer lane.mu.Unlock()
	if lane.circuitOpenTo.IsZero() || !lane.circuitOpenTo.After(now) {
		if !lane.circuitOpenTo.IsZero() {
			lane.circuitOpenTo = time.Time{}
			lane.failures = 0
		}
		return false, time.Time{}
	}
	return true, lane.circuitOpenTo
}

func laneAvailable(lane *proxyLaneRuntime, now time.Time, requireCapacity bool) bool {
	proxy, config, _, _ := lane.snapshot()
	if proxy == nil || !config.Enabled || !proxy.IsActive() || proxy.IsExpired(now) || proxy.URL() == "" {
		return false
	}
	if open, _ := laneCircuitOpen(lane, now); open {
		return false
	}
	return !requireCapacity || config.MaxConcurrency <= 0 || lane.active.Load() < int64(config.MaxConcurrency)
}

func laneScore(lane *proxyLaneRuntime) float64 {
	_, config, _, _ := lane.snapshot()
	maxConcurrency := max(1, config.MaxConcurrency)
	load := float64(lane.active.Load()) / float64(maxConcurrency)
	return load / float64(max(1, config.Weight))
}

func selectProxyLane(pool *accountProxyPoolRuntime, now time.Time, requireCapacity bool) *proxyLaneRuntime {
	if pool == nil {
		return nil
	}
	candidates := make([]*proxyLaneRuntime, 0, len(pool.lanes))
	for _, lane := range pool.lanes {
		if laneAvailable(lane, now, requireCapacity) {
			candidates = append(candidates, lane)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	switch pool.strategy {
	case ProxyLaneStrategyLeastConnections:
		best := candidates[0]
		for _, lane := range candidates[1:] {
			if laneScore(lane) < laneScore(best) {
				best = lane
			}
		}
		return best
	case ProxyLaneStrategyWeighted:
		total := 0
		for _, lane := range candidates {
			_, config, _, _ := lane.snapshot()
			total += max(1, config.Weight)
		}
		cursor := int(pool.counter.Add(1)-1) % total
		for _, lane := range candidates {
			_, config, _, _ := lane.snapshot()
			cursor -= max(1, config.Weight)
			if cursor < 0 {
				return lane
			}
		}
		return candidates[0]
	default:
		return candidates[int(pool.counter.Add(1)-1)%len(candidates)]
	}
}

// ResolveAccountProxyPoolURL keeps the legacy URL-only API for callers that
// already own the request lifecycle. HTTPUpstream uses AcquireProxyLane below
// so strict lane counters are held until the response body closes.
func ResolveAccountProxyPoolURL(accountID int64, fallback string) string {
	raw, ok := accountProxyPools.Load(accountID)
	if !ok {
		return fallback
	}
	pool, ok := raw.(*accountProxyPoolRuntime)
	if !ok || pool == nil {
		return fallback
	}
	if lane := selectProxyLane(pool, timeNow(), true); lane != nil {
		proxy, _, _, _ := lane.snapshot()
		return markAccountProxyLaneURL(proxy.URL(), accountID, proxy.ID)
	}
	return accountProxyLaneUnavailableURL
}

func markAccountProxyLaneURL(proxyURL string, accountID, proxyID int64) string {
	if proxyURL == "" || accountID <= 0 || proxyID <= 0 {
		return proxyURL
	}
	return proxyURL + proxyLaneURLMarker + strconv.FormatInt(accountID, 10) + ":" + strconv.FormatInt(proxyID, 10)
}

func splitAccountProxyLaneURL(value string) (string, int64, int64, bool) {
	index := strings.LastIndex(value, proxyLaneURLMarker)
	if index < 0 {
		return value, 0, 0, false
	}
	parts := strings.SplitN(value[index+len(proxyLaneURLMarker):], ":", 2)
	if len(parts) != 2 {
		return value, 0, 0, false
	}
	accountID, accountErr := strconv.ParseInt(parts[0], 10, 64)
	proxyID, proxyErr := strconv.ParseInt(parts[1], 10, 64)
	if accountErr != nil || proxyErr != nil || accountID <= 0 || proxyID <= 0 {
		return value, 0, 0, false
	}
	return value[:index], accountID, proxyID, true
}

// NextAccountProxyLaneURL returns a business-request marker understood by the
// shared HTTP and WebSocket transports. The marker never reaches the proxy.
func NextAccountProxyLaneURL(account *Account) string {
	if account == nil {
		return ""
	}
	if account.Proxy == nil && len(account.ProxyPool) == 0 && len(AccountProxyPoolIDs(account.Extra)) == 0 {
		return ""
	}
	if _, registered := accountProxyPools.Load(account.ID); !registered {
		RegisterAccountProxyPool(account)
		if _, registered = accountProxyPools.Load(account.ID); !registered {
			// Legacy primary-only accounts retain their original route. A lane is
			// activated once a pool or explicit lane config has been saved.
			if account.Proxy != nil && account.Proxy.URL() != "" {
				return account.Proxy.URL()
			}
		}
	}
	proxy := resolveAccountProxyPoolProxy(account)
	if proxy == nil {
		return accountProxyLaneUnavailableURL
	}
	return markAccountProxyLaneURL(proxy.URL(), account.ID, proxy.ID)
}

func resolveAccountProxyPoolProxy(account *Account) *Proxy {
	if account == nil {
		return nil
	}
	if raw, ok := accountProxyPools.Load(account.ID); ok {
		if pool, valid := raw.(*accountProxyPoolRuntime); valid {
			if lane := selectProxyLane(pool, timeNow(), true); lane != nil {
				proxy, _, _, _ := lane.snapshot()
				return proxy
			}
		}
	}
	candidates := healthyAccountProxyCandidates(append([]*Proxy{account.Proxy}, account.ProxyPool...), timeNow())
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
}

// AcquireAccountProxyLane atomically reserves capacity in one healthy lane.
// The returned release function must run after the upstream response is closed.
func AcquireAccountProxyLane(accountID int64, fallback string) (string, int64, int, func(bool)) {
	raw, ok := accountProxyPools.Load(accountID)
	if !ok {
		return fallback, 0, 0, func(bool) {}
	}
	pool, ok := raw.(*accountProxyPoolRuntime)
	if !ok || pool == nil {
		return fallback, 0, 0, func(bool) {}
	}

	var selected *proxyLaneRuntime
	for attempts := 0; attempts < len(pool.lanes)*2+1; attempts++ {
		lane := selectProxyLane(pool, timeNow(), true)
		if lane == nil {
			break
		}
		active := lane.active.Add(1)
		_, config, _, _ := lane.snapshot()
		if config.MaxConcurrency <= 0 || active <= int64(config.MaxConcurrency) {
			selected = lane
			break
		}
		lane.active.Add(-1)
	}
	if selected == nil {
		return "", 0, 0, func(bool) {}
	}
	var once sync.Once
	proxy, config, _, _ := selected.snapshot()
	return proxy.URL(), proxy.ID, config.TimeoutSeconds, func(success bool) {
		once.Do(func() {
			selected.active.Add(-1)
			selected.mu.Lock()
			defer selected.mu.Unlock()
			if success {
				selected.failures = 0
				selected.circuitOpenTo = time.Time{}
				return
			}
			selected.failures++
			threshold := selected.config.ErrorCircuitThreshold
			if threshold > 0 && selected.failures >= threshold {
				cooldown := max(1, selected.config.CircuitCooldownSeconds)
				selected.circuitOpenTo = timeNow().Add(time.Duration(cooldown) * time.Second)
			}
		})
	}
}

// AcquireAccountProxyLaneForURL reserves the concrete lane selected by
// Account.NextProxyURL. It uses CAS so concurrent requests cannot oversubscribe
// a lane between selection and transport setup.
func AcquireAccountProxyLaneForURL(accountID int64, requestedURL string) (string, int64, int, int, func(bool), error) {
	if requestedURL == accountProxyLaneUnavailableURL {
		return "", 0, 0, 0, func(bool) {}, ErrAccountProxyLaneUnavailable
	}
	plainURL, markedAccountID, requestedProxyID, marked := splitAccountProxyLaneURL(requestedURL)
	if !marked {
		return requestedURL, 0, 0, 0, func(bool) {}, nil
	}
	if accountID <= 0 {
		accountID = markedAccountID
	}
	if markedAccountID != accountID {
		return "", requestedProxyID, 0, 0, func(bool) {}, ErrAccountProxyLaneUnavailable
	}
	raw, ok := accountProxyPools.Load(accountID)
	if !ok {
		return plainURL, 0, 0, 0, func(bool) {}, nil
	}
	pool, ok := raw.(*accountProxyPoolRuntime)
	if !ok || pool == nil {
		return plainURL, 0, 0, 0, func(bool) {}, nil
	}
	var selected *proxyLaneRuntime
	for _, lane := range pool.lanes {
		proxy, _, _, _ := lane.snapshot()
		if proxy != nil && proxy.ID == requestedProxyID {
			selected = lane
			break
		}
	}
	if selected == nil || !laneAvailable(selected, timeNow(), true) {
		selected = selectProxyLane(pool, timeNow(), true)
	}
	if selected == nil {
		return "", requestedProxyID, 0, 0, func(bool) {}, ErrAccountProxyLaneUnavailable
	}
	selectedProxy, selectedConfig, _, _ := selected.snapshot()
	for {
		current := selected.active.Load()
		if selectedConfig.MaxConcurrency > 0 && current >= int64(selectedConfig.MaxConcurrency) {
			return "", selectedProxy.ID, selectedConfig.MaxConcurrency, selectedConfig.TimeoutSeconds, func(bool) {}, ErrAccountProxyLaneUnavailable
		}
		if selected.active.CompareAndSwap(current, current+1) {
			break
		}
	}
	var once sync.Once
	release := func(success bool) {
		once.Do(func() {
			selected.active.Add(-1)
			selected.mu.Lock()
			defer selected.mu.Unlock()
			if success {
				selected.failures = 0
				selected.circuitOpenTo = time.Time{}
				return
			}
			selected.failures++
			if threshold := selected.config.ErrorCircuitThreshold; threshold > 0 && selected.failures >= threshold {
				selected.circuitOpenTo = timeNow().Add(time.Duration(max(1, selected.config.CircuitCooldownSeconds)) * time.Second)
			}
		})
	}
	return selectedProxy.URL(), selectedProxy.ID, selectedConfig.MaxConcurrency, selectedConfig.TimeoutSeconds, release, nil
}

func AccountProxyLaneStatuses(account *Account) []ProxyLaneStatus {
	if account == nil {
		return nil
	}
	raw, ok := accountProxyPools.Load(account.ID)
	if !ok {
		RegisterAccountProxyPool(account)
		raw, ok = accountProxyPools.Load(account.ID)
	}
	if !ok {
		return nil
	}
	pool, ok := raw.(*accountProxyPoolRuntime)
	if !ok || pool == nil {
		return nil
	}
	now := timeNow()
	statuses := make([]ProxyLaneStatus, 0, len(pool.lanes))
	for _, lane := range pool.lanes {
		proxy, config, _, _ := lane.snapshot()
		if proxy == nil {
			continue
		}
		open, openUntil := laneCircuitOpen(lane, now)
		var circuitOpenUntil *time.Time
		if open {
			copy := openUntil
			circuitOpenUntil = &copy
		}
		primary := account.ProxyID != nil && *account.ProxyID == proxy.ID
		if account.ProxyID == nil && account.Proxy != nil {
			primary = account.Proxy.ID == proxy.ID
		}
		statuses = append(statuses, ProxyLaneStatus{
			ProxyID: proxy.ID, Name: proxy.Name,
			Primary:            primary,
			Enabled:            config.Enabled,
			Healthy:            laneAvailable(lane, now, false),
			CurrentConcurrency: int(lane.active.Load()), MaxConcurrency: config.MaxConcurrency,
			Weight: config.Weight, TimeoutSeconds: config.TimeoutSeconds,
			ErrorCircuitThreshold:  config.ErrorCircuitThreshold,
			CircuitCooldownSeconds: config.CircuitCooldownSeconds,
			FallbackOrder:          config.FallbackOrder, CircuitOpenUntil: circuitOpenUntil,
			Status: proxy.Status, ExpiresAt: proxy.ExpiresAt,
		})
	}
	return statuses
}

func healthyAccountProxyCandidates(proxies []*Proxy, now time.Time) []*Proxy {
	seen := make(map[int64]struct{}, len(proxies))
	candidates := make([]*Proxy, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil || proxy.ID <= 0 || !proxy.IsActive() || proxy.IsExpired(now) || proxy.URL() == "" {
			continue
		}
		if _, exists := seen[proxy.ID]; exists {
			continue
		}
		seen[proxy.ID] = struct{}{}
		candidates = append(candidates, proxy)
	}
	return candidates
}

var timeNow = func() time.Time { return time.Now() }

const accountProxyLaneUnavailableURL = "sub2api-proxy-lane://unavailable"
