package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/fleetdm/fleet/v4/server"
	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
)

type webhookLogWriter struct {
	url    string
	logger *slog.Logger
}

func NewWebhookLogWriter(ctx context.Context, webhookURL string, logger *slog.Logger) (*webhookLogWriter, error) {
	if webhookURL == "" {
		return nil, ctxerr.New(ctx, "webhook URL missing")
	}

	return &webhookLogWriter{
		url:    webhookURL,
		logger: logger,
	}, nil
}

type webhookPayload struct {
	Timestamp time.Time         `json:"timestamp"`
	Details   []json.RawMessage `json:"details"`
}

func (w *webhookLogWriter) Write(ctx context.Context, logs []json.RawMessage) error {

	payload := webhookPayload{
		Timestamp: time.Now(),
		Details:   logs,
	}

	w.logger.DebugContext(ctx, "sending webhook request",
		"url", server.MaskSecretURLParams(w.url),
	)

	if err := server.PostJSONWithTimeout(ctx, w.url, payload, w.logger); err != nil {
		// Intentionally swallow this error: webhook delivery failures are logged
		// here rather than returned so that a single failing webhook does not
		// interrupt or fail the broader logging pipeline for callers. See
		// TestWebhookFailure, which asserts Write returns nil in this case.
		w.logger.ErrorContext(ctx, fmt.Sprintf("failed to send automation webhook to %s", server.MaskSecretURLParams(w.url)),
			"err", server.MaskURLError(err).Error(),
		)
	}

	return nil
}
