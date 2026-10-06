package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// withChannelCacheCreationPolicy resolves the selected account's policy for one
// forwarding attempt. An explicit false shadows an earlier retry's true value.
// The request context is updated too: protocol converters use it when they do
// not receive a separate context argument.
func withChannelCacheCreationPolicy(ctx context.Context, c *gin.Context, channels *ChannelService, groupID int64, account *Account) (context.Context, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	enabled := false
	if channels != nil && account != nil && groupID > 0 {
		channel, err := channels.GetChannelForGroup(ctx, groupID)
		if err != nil {
			logger.LegacyPrintf("service.gateway", "failed to resolve cache creation policy: group_id=%d error=%v", groupID, err)
		} else if channel != nil {
			if override := channel.HideCacheCreationOverride(account.Platform); override != nil {
				enabled = *override
			}
		}
	}
	ctx = context.WithValue(ctx, cacheCreationVisibilityContextKey{}, enabled)
	if c != nil && c.Request != nil {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), cacheCreationVisibilityContextKey{}, enabled))
	}
	return ctx, enabled
}

func cacheCreationClientPolicyEnabled(c *gin.Context) bool {
	return c != nil && c.Request != nil && hideCacheCreationEnabled(c.Request.Context())
}

func sanitizeCacheCreationClientJSON(c *gin.Context, body []byte) []byte {
	if !cacheCreationClientPolicyEnabled(c) {
		return body
	}
	return sanitizeCacheCreationJSON(body)
}

// sanitizeCacheCreationClientSSE accepts a complete frame or a single SSE line.
// Only data JSON is normalized; event names, comments, [DONE], and framing stay
// intact. Callers must observe the original frame for accounting first.
func sanitizeCacheCreationClientSSE(c *gin.Context, frame string) string {
	if !cacheCreationClientPolicyEnabled(c) {
		return frame
	}
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		sanitized := sanitizeCacheCreationJSON([]byte(payload))
		if string(sanitized) != payload {
			lines[i] = "data: " + string(sanitized)
			if strings.HasSuffix(line, "\r") {
				lines[i] += "\r"
			}
		}
	}
	return strings.Join(lines, "\n")
}

func writeCacheCreationClientJSON(c *gin.Context, status int, value any) {
	if !cacheCreationClientPolicyEnabled(c) {
		c.JSON(status, value)
		return
	}
	body, err := json.Marshal(value)
	if err != nil {
		_ = c.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Data(status, "application/json; charset=utf-8", sanitizeCacheCreationJSON(body))
}
