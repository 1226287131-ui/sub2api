//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the real exported forwarding boundaries, including channel lookup,
// each protocol converter, final usage frames, and the immutable billing result.
func TestCacheCreationPolicyForwardingContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat", "messages"} {
		for _, upstreamProtocol := range []string{"raw_chat", "responses", "native_anthropic"} {
			for _, stream := range []bool{false, true} {
				for _, enabled := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/stream=%v/enabled=%v", endpoint, upstreamProtocol, stream, enabled), func(t *testing.T) {
						account := rawChatCompletionsTestAccount()
						account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: upstreamProtocol != "raw_chat"}
						model := "gpt-5.4"
						if upstreamProtocol == "native_anthropic" {
							account = nativeAnthropicTestAccount()
							model = "k3"
						}
						path := "/v1/chat/completions"
						if endpoint == "messages" {
							path = "/v1/messages"
						}
						body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":%v,"stream_options":{"include_usage":true}}`, model, stream))
						recorder := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(recorder)
						c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
						c.Request.Header.Set("Content-Type", "application/json")
						groupID := int64(17)
						c.Set("api_key", &APIKey{ID: 12, GroupID: &groupID})
						channels := cacheVisibilityTestChannel(groupID, account.Platform, enabled)
						// Converters always ask the native Anthropic / Responses
						// upstream for SSE, even for a buffered client response.
						upstreamStream := stream || upstreamProtocol == "responses" || (upstreamProtocol == "native_anthropic" && endpoint == "chat")
						upstream := &httpUpstreamRecorder{resp: cacheVisibilityUpstreamResponse(upstreamProtocol, model, upstreamStream)}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), channelService: channels, httpUpstream: upstream}
						var result *OpenAIForwardResult
						var err error
						if endpoint == "chat" {
							result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
						} else {
							result, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
						}
						require.NoError(t, err)
						require.NotNil(t, result)
						require.NotNil(t, result.CacheCreationAsInput)
						assert.Equal(t, enabled, *result.CacheCreationAsInput)
						assert.Equal(t, OpenAIUsage{InputTokens: 12, OutputTokens: 3, CacheReadInputTokens: 4, CacheCreationInputTokens: 6}, result.Usage, "do not mutate upstream evidence used by settlement")
						assert.Equal(t, http.StatusOK, recorder.Code)
						if !stream {
							assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
						}
						client := recorder.Body.String()
						if enabled {
							assert.NotContains(t, client, "cache_creation")
							assert.NotContains(t, client, "cached_creation")
							assert.NotContains(t, client, "cache_write")
						} else {
							assert.True(t, strings.Contains(client, "cache_creation") || strings.Contains(client, "cached_creation") || strings.Contains(client, "cache_write"), "disabled policy must preserve creation usage: %s", client)
						}
						wantInput := int64(12) // OpenAI total already includes writes.
						inputKey := "prompt_tokens"
						if endpoint == "messages" {
							inputKey = "input_tokens"
							wantInput = 2
							if enabled {
								wantInput = 8 // two ordinary + six creation, read stays four.
							}
						}
						var seenInput, seenRead bool
						payloads := []string{client}
						if stream {
							payloads = nil
							for _, line := range strings.Split(client, "\n") {
								if payload, ok := extractOpenAISSEDataLine(line); ok {
									payloads = append(payloads, payload)
								}
							}
						}
						for _, payload := range payloads {
							for _, usagePath := range []string{"usage", "message.usage"} {
								usage := gjson.Get(payload, usagePath)
								if value := usage.Get(inputKey); value.Exists() && value.Int() == wantInput {
									seenInput = true
								}
								if usage.Get("prompt_tokens_details.cached_tokens").Int() == 4 || usage.Get("cache_read_input_tokens").Int() == 4 {
									seenRead = true
								}
							}
						}
						assert.True(t, seenInput, "client must see %s=%d: %s", inputKey, wantInput, client)
						assert.True(t, seenRead, "cache reads must remain unchanged: %s", client)
					})
				}
			}
		}
	}
}

func cacheVisibilityTestChannel(groupID int64, platform string, enabled bool) *ChannelService {
	cache := newEmptyChannelCache()
	cache.loadedAt = time.Now()
	cache.channelByGroupID[groupID] = &Channel{ID: 9, Status: StatusActive, FeaturesConfig: map[string]any{
		featureKeyHideCacheCreation: map[string]any{platform: enabled},
	}}
	channels := &ChannelService{}
	channels.cache.Store(cache)
	return channels
}

func cacheVisibilityUpstreamResponse(protocol, model string, stream bool) *http.Response {
	contentType := "application/json"
	var body string
	switch protocol {
	case "raw_chat":
		usage := `{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":4,"cache_write_tokens":6}}`
		body = fmt.Sprintf(`{"id":"chat_cache","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":%s}`, model, usage)
		if stream {
			body = fmt.Sprintf("data: {\"id\":\"chat_cache\",\"object\":\"chat.completion.chunk\",\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"id\":\"chat_cache\",\"object\":\"chat.completion.chunk\",\"model\":%q,\"choices\":[],\"usage\":%s}\n\ndata: [DONE]\n\n", model, model, usage)
		}
	case "responses":
		body = fmt.Sprintf("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_cache\",\"model\":%q,\"status\":\"in_progress\",\"output\":[]}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_cache\",\"object\":\"response\",\"model\":%q,\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"msg_cache\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":12,\"output_tokens\":3,\"total_tokens\":15,\"input_tokens_details\":{\"cached_tokens\":4,\"cached_creation_tokens\":6}}}}\n\ndata: [DONE]\n\n", model, model)
	case "native_anthropic":
		body = fmt.Sprintf(`{"id":"msg_cache","type":"message","role":"assistant","model":%q,"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":4,"cache_creation_input_tokens":6}}`, model)
		if stream {
			body = fmt.Sprintf("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_cache\",\"type\":\"message\",\"role\":\"assistant\",\"model\":%q,\"content\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1,\"cache_read_input_tokens\":4,\"cache_creation_input_tokens\":6}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", model)
		}
	}
	if stream {
		contentType = "text/event-stream"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"cache-contract"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCacheCreationPolicyRetryResolvesSelectedPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	channels := cacheVisibilityTestChannel(17, PlatformOpenAI, true)
	ctx, enabled := withChannelCacheCreationPolicy(context.Background(), c, channels, 17, &Account{Platform: PlatformOpenAI})
	require.True(t, enabled)
	assert.True(t, cacheCreationClientPolicyEnabled(c))
	ctx, enabled = withChannelCacheCreationPolicy(ctx, c, channels, 17, &Account{Platform: PlatformAnthropic})
	assert.False(t, enabled)
	assert.False(t, hideCacheCreationEnabled(ctx), "retry to a platform without the toggle must clear the old flag")
	assert.False(t, cacheCreationClientPolicyEnabled(c))
}

func TestCacheCreationPolicyWebSocketClientFrames(t *testing.T) {
	const upstream = `{"type":"response.completed","response":{"id":"resp_ws_cache","usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15,"input_tokens_details":{"cached_tokens":4,"cached_creation_tokens":6}}}}`
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			inner := &fakePassthroughFrameConn{}
			conn := &openAIWSPolicyEnforcingFrameConn{inner: inner, cacheCreationAsInput: enabled}
			original := []byte(upstream)
			require.NoError(t, conn.WriteFrame(context.Background(), coderws.MessageText, original))
			require.Len(t, inner.writes, 1)
			assert.Equal(t, upstream, string(original), "relay accounting observes the immutable upstream frame")
			usage := gjson.GetBytes(inner.writes[0], "response.usage")
			assert.Equal(t, int64(12), usage.Get("input_tokens").Int())
			assert.Equal(t, int64(4), usage.Get("input_tokens_details.cached_tokens").Int())
			assert.Equal(t, !enabled, usage.Get("input_tokens_details.cached_creation_tokens").Exists())
			if !enabled {
				assert.Equal(t, upstream, string(inner.writes[0]))
			}
		})
	}
}
