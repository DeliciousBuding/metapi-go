package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (op directMediaOperation) finishModelScope(ctx context.Context, submission *http.Request, resp *http.Response, payload map[string]json.RawMessage) (result *http.Response, err error) {
	defer func() {
		if err != nil {
			body, _ := json.Marshal(payload)
			if usage := ParseUsageFromBody(body); usage.Found {
				err = &directMediaTransportError{cause: err, usage: usage}
			}
		}
	}()
	taskID := mediaString(payload, "task_id")
	var taskURL string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		state := strings.ToUpper(strings.TrimSpace(mediaString(payload, "task_status")))
		if state == "FAILED" || state == "CANCELED" || state == "CANCELLED" || len(payload["errors"]) > 0 && string(payload["errors"]) != "null" || len(payload["error"]) > 0 && string(payload["error"]) != "null" {
			code, message := modelScopeFailure(payload)
			return directMediaFailure(resp, payload, code, message), nil
		}
		if state == "SUCCEED" || state == "SUCCESS" || state == "" && (len(payload["data"]) > 0 || len(payload["output_images"]) > 0) {
			return op.modelScopeImages(ctx, resp, payload)
		}
		if taskID == "" {
			return directMediaFailure(resp, payload, "missing_task", "ModelScope response has no images or task_id"), nil
		}
		if state != "" && state != "PENDING" && state != "RUNNING" && state != "QUEUED" && state != "PROCESSING" {
			return directMediaFailure(resp, payload, "invalid_task_status", "ModelScope returned an unknown task status"), nil
		}
		if taskURL == "" {
			var err error
			taskURL, err = modelScopeTaskURL(submission.URL, taskID)
			if err != nil {
				return directMediaFailure(resp, payload, "invalid_task", err.Error()), nil
			}
		} else {
			timer := time.NewTimer(op.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		query, err := http.NewRequestWithContext(ctx, http.MethodGet, taskURL, nil)
		if err != nil {
			return nil, err
		}
		query.Header = submission.Header.Clone()
		for _, key := range []string{"Content-Type", "Content-Length", "X-ModelScope-Async-Mode"} {
			query.Header.Del(key)
		}
		query.Header.Set("Accept", "application/json")
		query.Header.Set("X-ModelScope-Task-Type", "image_generation")
		// Authentication is copied from the actual submitting request. Never
		// resolve the selected credential a second time during this operation.
		poll, err := sendUpstreamRequest(op.cfg, query, op.proxyConfig, op.firstByteTimeoutMs, false)
		if err != nil {
			return nil, err
		}
		body, err := readDirectMediaResponse(poll)
		if err != nil {
			return nil, err
		}
		if poll.StatusCode < 200 || poll.StatusCode >= 300 {
			return carryDirectMediaUsage(poll, body, payload), nil
		}
		next, err := decodeDirectMediaResponse(poll, body)
		if err != nil {
			return directMediaFailure(poll, payload, "invalid_response", err.Error()), nil
		}
		// Retain actual usage/metadata from submission when the task endpoint
		// omits them. New observations take precedence; never synthesize tokens.
		for _, key := range []string{"usage", "metadata", "id", "task_id", "created"} {
			if _, exists := next[key]; !exists && len(payload[key]) > 0 {
				next[key] = payload[key]
			}
		}
		payload, resp = next, poll
	}
}

func modelScopeTaskURL(submission *url.URL, taskID string) (string, error) {
	if taskID == "." || taskID == ".." || !strings.HasSuffix(submission.Path, "/images/generations") {
		return "", fmt.Errorf("cannot derive ModelScope task URL from the configured endpoint")
	}
	task := *submission
	task.Path = strings.TrimSuffix(submission.Path, "/images/generations") + "/tasks/" + taskID
	task.RawPath = strings.TrimSuffix(submission.EscapedPath(), "/images/generations") + "/tasks/" + url.PathEscape(taskID)
	return task.String(), nil
}

func modelScopeFailure(payload map[string]json.RawMessage) (any, string) {
	code := any("task_failed")
	if raw := payload["code"]; len(raw) > 0 {
		code = raw
	}
	message := mediaString(payload, "message")
	for _, key := range []string{"errors", "error"} {
		var details map[string]json.RawMessage
		if json.Unmarshal(payload[key], &details) == nil {
			if raw := details["code"]; len(raw) > 0 {
				code = raw
			}
			if message == "" {
				message = mediaString(details, "message")
			}
		}
	}
	if message == "" {
		message = "ModelScope image task failed"
	}
	return code, message
}
