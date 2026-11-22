package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	requestTimeout = 45 * time.Second
	maxJSON        = 16 << 20
	maxTranscript  = 32 << 20
)

type credentialsError struct{ message string }

func (e credentialsError) Error() string { return e.message }

type statusError struct {
	operation string
	code      int
}

func (e statusError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.operation, e.code) }

func hasStatus(err error, code int) bool {
	var status statusError
	return errors.As(err, &status) && status.code == code
}

func request(ctx context.Context, client *http.Client, operation, method, endpoint string, headers http.Header, body []byte, attempts int, limit int64) (*http.Response, []byte, error) {
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: invalid request", operation)
		}
		if headers != nil {
			req.Header = headers.Clone()
		}
		resp, err := client.Do(req)
		if err == nil {
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				if attempt+1 < attempts {
					resp.Body.Close()
					if err := wait(ctx, time.Second<<attempt); err != nil {
						return nil, nil, err
					}
					continue
				}
			}
			content, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
			resp.Body.Close()
			if readErr == nil && int64(len(content)) <= limit {
				return resp, content, nil
			}
			if int64(len(content)) > limit {
				return nil, nil, fmt.Errorf("%s: response exceeds %d bytes", operation, limit)
			}
		} else {
			var auth credentialsError
			if errors.As(err, &auth) {
				return nil, nil, auth
			}
		}
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if attempt+1 < attempts {
			if err := wait(ctx, time.Second<<attempt); err != nil {
				return nil, nil, err
			}
		}
	}
	return nil, nil, fmt.Errorf("%s: transport or response read failed", operation)
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
