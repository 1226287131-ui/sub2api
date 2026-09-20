package service

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

type proxyLaneResponseBody struct {
	body    io.ReadCloser
	release func(bool)
	cancel  context.CancelFunc
	success bool
	once    sync.Once
}

func (b *proxyLaneResponseBody) Read(p []byte) (int, error) { return b.body.Read(p) }
func (b *proxyLaneResponseBody) Close() error {
	err := b.body.Close()
	b.once.Do(func() {
		b.release(b.success && err == nil)
		b.cancel()
	})
	return err
}

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream 只在 OpenAI OAuth 能力绑定已启用时把真实请求交给插件。
// 插件返回标准 http.Response，响应解析、错误映射、SSE 和计费仍由现有核心链处理。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	requestCtx := request.Context()
	if !AccountProxyPoolResolved(requestCtx) {
		proxyURL = account.NextProxyLaneURL()
		requestCtx = WithAccountProxyPoolResolved(requestCtx)
	}
	request = request.WithContext(WithAccountProxyPoolEligible(requestCtx))
	if s.pluginManager != nil && s.pluginManager.ShouldRouteOpenAIOAuth(account) {
		plainProxyURL, _, _, laneTimeout, releaseLane, laneErr := AcquireAccountProxyLaneForURL(account.ID, proxyURL)
		if laneErr != nil {
			return nil, laneErr
		}
		pluginRequest := request
		cancelLaneTimeout := func() {}
		if laneTimeout > 0 {
			var cancel context.CancelFunc
			requestCtx, cancel = context.WithTimeout(request.Context(), time.Duration(laneTimeout)*time.Second)
			pluginRequest = request.Clone(requestCtx)
			cancelLaneTimeout = cancel
		}
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(pluginRequest.Context(), pluginRequest, plainProxyURL, account)
		if handled {
			if err != nil {
				releaseLane(false)
				cancelLaneTimeout()
				return response, err
			}
			if response == nil || response.Body == nil {
				releaseLane(false)
				cancelLaneTimeout()
				return response, err
			}
			response.Body = &proxyLaneResponseBody{
				body: response.Body, release: releaseLane, cancel: cancelLaneTimeout,
				success: response.StatusCode < http.StatusInternalServerError && response.StatusCode != http.StatusTooManyRequests,
			}
			if err == nil {
				s.captureOpenAICodexTicketFromUpstream(pluginRequest, account, response)
			}
			return response, err
		}
		releaseLane(true)
		cancelLaneTimeout()
	}
	response, err := s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
	if err == nil {
		s.captureOpenAICodexTicketFromUpstream(request, account, response)
	}
	return response, err
}

// doOpenAIAccountTestUpstream 让 OpenAI OAuth 账号测试与真实转发使用同一插件路径。
// API Key 和未命中插件的账号保持各自原有的 HTTPUpstream 行为。
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
) (*http.Response, error) {
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if useTLSFallback {
		return s.httpUpstream.DoWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}
