package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pluginRoutingHTTPUpstream struct {
	doCalls        int
	doWithTLSCalls int
	lastRequest    *http.Request
}

func (u *pluginRoutingHTTPUpstream) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.doCalls++
	u.lastRequest = request
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("legacy")),
	}, nil
}

func (u *pluginRoutingHTTPUpstream) DoWithTLS(
	request *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	u.doWithTLSCalls++
	return u.Do(request, proxyURL, accountID, accountConcurrency)
}

func TestPluginManagerRoutingDoesNotTouchAPIKeyOrOtherProviders(t *testing.T) {
	manager := &PluginManager{}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", nil)
	require.NoError(t, err)

	accounts := []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		{ID: 3, Platform: PlatformGemini, Type: AccountTypeOAuth},
	}
	for _, account := range accounts {
		response, handled, routeErr := manager.RoundTripOpenAIOAuth(context.Background(), request, "", account)
		assert.Nil(t, response)
		assert.False(t, handled)
		assert.NoError(t, routeErr)
	}
}

func TestPluginManagerRoutingKeepsOAuthOnLegacyPathWithoutEnabledBinding(t *testing.T) {
	manager := &PluginManager{}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", nil)
	require.NoError(t, err)
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	response, handled, routeErr := manager.RoundTripOpenAIOAuth(context.Background(), request, "", account)

	assert.Nil(t, response)
	assert.False(t, handled)
	assert.NoError(t, routeErr)
}

func TestPluginManagerRoutingSelectsOnlyEligibleOpenAIOAuthAccounts(t *testing.T) {
	manager := &PluginManager{}
	manager.route.Store(&pluginRoute{pluginID: 1, rolloutPercent: 100, unavailable: "测试不可用"})

	assert.True(t, manager.ShouldRouteOpenAIOAuth(&Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
	assert.False(t, manager.ShouldRouteOpenAIOAuth(&Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}))
	assert.False(t, manager.ShouldRouteOpenAIOAuth(&Account{ID: 10, Platform: PlatformGrok, Type: AccountTypeOAuth}))
	assert.False(t, manager.ShouldRouteOpenAIOAuth(nil))
}

func TestOpenAIGatewayPluginRoutingPreservesAPIKeyAndFailsClosedForOAuth(t *testing.T) {
	manager := &PluginManager{}
	manager.route.Store(&pluginRoute{pluginID: 1, rolloutPercent: 100, unavailable: "测试不可用"})
	upstream := &pluginRoutingHTTPUpstream{}
	service := &OpenAIGatewayService{pluginManager: manager, httpUpstream: upstream}

	apiKeyRequest, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", nil)
	require.NoError(t, err)
	apiKeyResponse, err := service.doOpenAIUpstream(apiKeyRequest, "", &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, apiKeyResponse)
	_ = apiKeyResponse.Body.Close()
	assert.Equal(t, 1, upstream.doCalls)

	oauthRequest, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", nil)
	require.NoError(t, err)
	oauthResponse, err := service.doOpenAIUpstream(oauthRequest, "", &Account{
		ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
	})
	require.Error(t, err)
	assert.Nil(t, oauthResponse)
	assert.Contains(t, err.Error(), "插件不可用")
	assert.Equal(t, 1, upstream.doCalls)
}

type pluginRoutingDisableOnDeadlineContext struct {
	context.Context
	manager *PluginManager
	once    sync.Once
}

func (c *pluginRoutingDisableOnDeadlineContext) Deadline() (time.Time, bool) {
	// The lane timeout queries Deadline after the first route check, allowing
	// the binding change before RoundTrip's second check without timing a race.
	c.once.Do(func() { c.manager.route.Store(nil) })
	return c.Context.Deadline()
}

func TestOpenAIGatewayPluginRoutingChangePreservesFallbackRequestContext(t *testing.T) {
	manager := &PluginManager{}
	manager.route.Store(&pluginRoute{pluginID: 1, rolloutPercent: 100, unavailable: "route will be disabled"})
	account := &Account{
		ID: 92001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Proxy: testAccountProxy(1, StatusActive, nil),
		Extra: map[string]any{
			AccountProxyLaneConfigsExtraKey: []ProxyLaneConfig{
				{ProxyID: 1, Enabled: true, MaxConcurrency: 1, TimeoutSeconds: 30},
			},
		},
	}
	RegisterAccountProxyPool(account)
	t.Cleanup(func() { accountProxyPools.Delete(account.ID) })
	require.True(t, manager.ShouldRouteOpenAIOAuth(account))
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	ctx := &pluginRoutingDisableOnDeadlineContext{Context: parentCtx, manager: manager}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", strings.NewReader(`{"model":"gpt-5.5"}`))
	require.NoError(t, err)
	upstream := &pluginRoutingHTTPUpstream{}
	service := &OpenAIGatewayService{pluginManager: manager, httpUpstream: upstream}

	response, err := service.doOpenAIUpstream(request, "", account)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.NoError(t, response.Body.Close())
	assert.False(t, manager.ShouldRouteOpenAIOAuth(account))
	assert.Equal(t, 1, upstream.doCalls)
	require.NotNil(t, upstream.lastRequest)
	assert.NoError(t, upstream.lastRequest.Context().Err())
	_, hasDeadline := upstream.lastRequest.Context().Deadline()
	assert.False(t, hasDeadline, "the plugin lane timeout must not become the HTTP fallback deadline")
	assert.True(t, AccountProxyPoolEligible(upstream.lastRequest.Context()))
	assert.True(t, AccountProxyPoolResolved(upstream.lastRequest.Context()))
	body, err := io.ReadAll(upstream.lastRequest.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"model":"gpt-5.5"}`, string(body))
	statuses := AccountProxyLaneStatuses(account)
	require.Len(t, statuses, 1)
	assert.Zero(t, statuses[0].CurrentConcurrency, "the declined plugin attempt must release its lane")
	cancelParent()
	assert.ErrorIs(t, upstream.lastRequest.Context().Err(), context.Canceled, "fallback must retain caller cancellation")
}

func TestStablePluginBucketIsDeterministicAndBounded(t *testing.T) {
	for id := int64(1); id <= 1000; id++ {
		first := stablePluginBucket(id)
		assert.Equal(t, first, stablePluginBucket(id))
		assert.Less(t, first, uint64(100))
	}
}

type statusStubRepository struct {
	PluginRepository
}

func (r *statusStubRepository) GetByID(context.Context, int64) (*PluginInstallation, error) {
	return &PluginInstallation{ID: 7}, nil
}

// Status is the read-only, ungated runtime-status channel: when the plugin is not
// running it must report "not running" with no status blob and never error or start
// a runtime (that side-effecting behaviour belongs to Test, not Status).
func TestPluginManagerStatusReportsNotRunningWithoutRuntime(t *testing.T) {
	manager := &PluginManager{repo: &statusStubRepository{}}
	resp, err := manager.Status(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Healthy)
	assert.Equal(t, "插件未运行", resp.Message)
	assert.Empty(t, resp.StatusJson)
}
