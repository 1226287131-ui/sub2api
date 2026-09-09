package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	openAIStreamPrimingContextKey = "openai_stream_priming_sent"
	openAIStreamPrimingFrame      = "event: ping\ndata: {\"type\":\"ping\"}\n\n"
)

func (s *OpenAIGatewayService) openAIStreamPrimingEnabled(account *Account) bool {
	return s.openAIStreamPrimingConfigured() && account != nil && account.IsOpenAICompatible()
}

func (s *OpenAIGatewayService) openAIStreamPrimingConfigured() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIStreamPrimingEnabled
}

// SendOpenAIStreamPrimingEarly commits the non-semantic priming frame before
// account selection and concurrency-slot waits. It deliberately does not
// require an account: the downstream client should receive an immediate SSE
// byte even while every eligible upstream account is busy.
func (s *OpenAIGatewayService) SendOpenAIStreamPrimingEarly(c *gin.Context) bool {
	if !s.openAIStreamPrimingConfigured() || c == nil || c.Writer == nil {
		return false
	}
	return s.writeOpenAIStreamPriming(c)
}

// sendOpenAIStreamPriming commits one non-semantic SSE frame early enough for a
// downstream NewAPI StreamScannerHandler to record its first-response time.
// The bytes are recorded as keepalive data so sub2API's failover accounting
// still treats the request as pre-semantic-output.
func (s *OpenAIGatewayService) sendOpenAIStreamPriming(c *gin.Context, account *Account) bool {
	if !s.openAIStreamPrimingEnabled(account) || c == nil || c.Writer == nil {
		return false
	}
	return s.writeOpenAIStreamPriming(c)
}

func (s *OpenAIGatewayService) writeOpenAIStreamPriming(c *gin.Context) bool {
	if sent, ok := c.Get(openAIStreamPrimingContextKey); ok {
		if value, valid := sent.(bool); valid && value {
			return false
		}
	}
	// Do not append a priming frame to a response that another middleware or
	// protocol bridge has already started.  In particular, a compact SSE
	// keepalive may have committed its own response before this hook runs.
	if c.Writer.Written() {
		return false
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return false
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	n, err := c.Writer.Write([]byte(openAIStreamPrimingFrame))
	recordOpenAIStreamKeepaliveBytes(c, n)
	if err != nil {
		return false
	}
	c.Set(openAIStreamPrimingContextKey, true)
	flusher.Flush()
	return true
}
