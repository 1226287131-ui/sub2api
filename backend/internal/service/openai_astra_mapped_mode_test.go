//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForward_AstraMappedReasoningModeUsesUpstreamModel(t *testing.T) {
	for _, tc := range []struct {
		name       string
		requested  string
		upstream   string
		mode       string
		effort     string
		wantMode   string
		wantEffort string
	}{
		{"alias to Astra preserves pro", "private-model", "gpt-6-astra", "pro", "", "pro", ""},
		{"alias preserves separate effort", "private-model", "gpt-6-astra", "pro", "high", "pro", "high"},
		{"alias preserves standard mode", "private-model", "gpt-6-astra", "standard", "max", "standard", "max"},
		{"Astra mapped to legacy keeps compatibility", "gpt-6-astra", "gpt-5.6-sol", "pro", "", "", "max"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newAstraOAuthSetup(t, false)
			s.account.Credentials["model_mapping"] = map[string]any{tc.requested: tc.upstream}
			inner := `{"id":"resp_mapping","model":"` + tc.upstream + `","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
			s.upstream.resp = &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(codexCompletedSSE(inner))),
			}

			result, err := s.svc.Forward(context.Background(), s.c, s.account, astraRequestBody(tc.requested, true, tc.mode, tc.effort))

			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, s.upstream.lastReq)
			assert.Equal(t, tc.upstream, gjson.GetBytes(s.upstream.lastBody, "model").String())
			assert.Equal(t, tc.wantMode, gjson.GetBytes(s.upstream.lastBody, "reasoning.mode").String())
			assert.Equal(t, tc.wantEffort, gjson.GetBytes(s.upstream.lastBody, "reasoning.effort").String())
		})
	}
}
