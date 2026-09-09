package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIStreamPrimingTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	return c, recorder
}

func TestSendOpenAIStreamPriming_Disabled(t *testing.T) {
	c, recorder := newOpenAIStreamPrimingTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{}}}

	require.False(t, svc.sendOpenAIStreamPriming(c, &Account{Platform: PlatformOpenAI}))
	require.Empty(t, recorder.Body.String())
}

func TestSendOpenAIStreamPrimingEarly_DoesNotRequireAccount(t *testing.T) {
	c, recorder := newOpenAIStreamPrimingTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamPrimingEnabled: true,
	}}}

	require.True(t, svc.SendOpenAIStreamPrimingEarly(c))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, openAIStreamPrimingFrame, recorder.Body.String())
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
	require.False(t, openAIStreamClientOutputStarted(c, false))
	require.False(t, svc.SendOpenAIStreamPrimingEarly(c), "the frame must be emitted once per request")
}

func TestSendOpenAIStreamPriming_RecordsNonSemanticFrameOnly(t *testing.T) {
	c, recorder := newOpenAIStreamPrimingTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamPrimingEnabled: true,
	}}}
	account := &Account{Platform: PlatformOpenAI}

	require.True(t, svc.sendOpenAIStreamPriming(c, account))
	require.Equal(t, openAIStreamPrimingFrame, recorder.Body.String())
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
	require.False(t, openAIStreamClientOutputStarted(c, false))

	// A failover retry must not prepend another priming frame to the same client stream.
	require.False(t, svc.sendOpenAIStreamPriming(c, account))
	require.Equal(t, openAIStreamPrimingFrame, recorder.Body.String())
}

func TestSendOpenAIStreamPriming_RequiresOpenAICompatibleAccount(t *testing.T) {
	c, recorder := newOpenAIStreamPrimingTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamPrimingEnabled: true,
	}}}

	require.False(t, svc.sendOpenAIStreamPriming(c, &Account{Platform: PlatformAnthropic}))
	require.Empty(t, recorder.Body.String())
}

func TestSendOpenAIStreamPriming_DoesNotAppendToCommittedResponse(t *testing.T) {
	c, recorder := newOpenAIStreamPrimingTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamPrimingEnabled: true,
	}}}

	_, _ = c.Writer.WriteString(": keepalive\n\n")
	require.False(t, svc.SendOpenAIStreamPrimingEarly(c))
	require.Equal(t, ": keepalive\n\n", recorder.Body.String())
}
