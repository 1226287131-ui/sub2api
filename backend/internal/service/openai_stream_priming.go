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
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIStreamPrimingEnabled &&
		account != nil && account.IsOpenAICompatible()
}

// sendOpenAIStreamPriming commits one non-semantic SSE frame early enough for a
// downstream NewAPI StreamScannerHandler to record its first-response time.
// The bytes are recorded as keepalive data so sub2API's failover accounting
// still treats the request as pre-semantic-output.
func (s *OpenAIGatewayService) sendOpenAIStreamPriming(c *gin.Context, account *Account) bool {
	if !s.openAIStreamPrimingEnabled(account) || c == nil || c.Writer == nil {
		return false
	}
	if sent, ok := c.Get(openAIStreamPrimingContextKey); ok {
		if value, valid := sent.(bool); valid && value {
			return false
		}
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return false
	}
	n, err := c.Writer.Write([]byte(openAIStreamPrimingFrame))
	recordOpenAIStreamKeepaliveBytes(c, n)
	if err != nil {
		return false
	}
	c.Set(openAIStreamPrimingContextKey, true)
	flusher.Flush()
	return true
}
