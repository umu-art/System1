package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	mtsGateway = "https://gw.mts-link.ru"
	mtsOrigin  = "https://my.mts-link.ru"
)

type room struct {
	name      string
	sessionID string
}

type transcript struct {
	ID        int64     `json:"transcriptId"`
	CreatedAt time.Time `json:"createdAt"`
}

type mtsSession struct {
	access, refresh               string
	accessExpires, refreshExpires time.Time
}

type mtsClient struct {
	gateway string
	http    *http.Client
	session mtsSession
	save    func(context.Context, mtsSession) error
}

func newMTSClient(session mtsSession, save func(context.Context, mtsSession) error) *mtsClient {
	return &mtsClient{
		gateway: mtsGateway, session: session, save: save,
		http: &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Scheme != "https" || len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				if req.URL.Host != via[0].URL.Host {
					req.Header.Del("Cookie")
					req.Header.Del("Origin")
				}
				return nil
			},
		},
	}
}

func (m *mtsClient) headers() http.Header {
	req := &http.Request{Header: make(http.Header)}
	if m.session.access != "" {
		req.AddCookie(&http.Cookie{Name: "access", Value: m.session.access})
	}
	req.AddCookie(&http.Cookie{Name: "refresh", Value: m.session.refresh})
	req.Header.Set("Origin", mtsOrigin)
	return req.Header
}

func (m *mtsClient) prepare(ctx context.Context) error {
	if m.session.refresh == "" || (!m.session.refreshExpires.IsZero() && time.Now().After(m.session.refreshExpires)) {
		return credentialsError{"MTS_LINK_AUTH: refresh cookie is missing or expired"}
	}
	if m.session.access == "" || m.session.accessExpires.Before(time.Now().Add(5*time.Minute)) {
		return m.refresh(ctx)
	}
	m.logExpiry()
	return nil
}

func (m *mtsClient) refresh(ctx context.Context) error {
	op := "refresh MTS Link session"
	headers := m.headers()
	headers.Set("Content-Type", "application/json")
	client := *m.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, _, err := request(ctx, &client, op, http.MethodPost, m.gateway+"/accountUcaas/AccountUcaas.BrowserRefresh", headers, []byte("{}"), 1, maxJSON)
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return credentialsError{"MTS_LINK_AUTH: refresh rejected by MTS Link"}
	case http.StatusOK:
	default:
		return statusError{op, resp.StatusCode}
	}
	next := m.session
	gotAccess := false
	for _, cookie := range resp.Cookies() {
		switch cookie.Name {
		case "access":
			next.access, next.accessExpires = cookie.Value, cookieExpiry(cookie)
			gotAccess = cookie.Value != "" && cookie.MaxAge >= 0
		case "refresh":
			next.refresh, next.refreshExpires = cookie.Value, cookieExpiry(cookie)
			if cookie.MaxAge < 0 {
				next.refresh = ""
			}
		}
	}
	if !gotAccess || next.refresh == "" {
		return credentialsError{"MTS_LINK_AUTH: refresh response lacks usable session cookies"}
	}
	if err := m.save(ctx, next); err != nil {
		return fmt.Errorf("persist rotated MTS Link cookies: %w", err)
	}
	m.session = next
	m.logExpiry()
	return nil
}

func (m *mtsClient) logExpiry() {
	slog.Info("MTS Link session", "access_expires_at", formatExpiry(m.session.accessExpires), "refresh_expires_at", formatExpiry(m.session.refreshExpires))
}

func cookieExpiry(cookie *http.Cookie) time.Time {
	if !cookie.Expires.IsZero() {
		return cookie.Expires
	}
	if cookie.MaxAge > 0 {
		return time.Now().Add(time.Duration(cookie.MaxAge) * time.Second)
	}
	return jwtExpiry(cookie.Value)
}

func jwtExpiry(value string) time.Time {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp <= 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

func (m *mtsClient) get(ctx context.Context, operation, path, accept string, limit int64) (*http.Response, []byte, error) {
	get := func() (*http.Response, []byte, error) {
		headers := m.headers()
		headers.Set("Accept", accept)
		return request(ctx, m.http, operation, http.MethodGet, m.gateway+path, headers, nil, 3, limit)
	}
	resp, data, err := get()
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		if err := m.refresh(ctx); err != nil {
			return nil, nil, err
		}
		resp, data, err = get()
	}
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, nil, credentialsError{"MTS_LINK_AUTH: access to transcripts was denied"}
	}
	return resp, data, nil
}

func (m *mtsClient) list(ctx context.Context, source room, createdFrom time.Time) ([]transcript, error) {
	op := "list MTS Link transcripts"
	var all []transcript
	cursors := map[string]bool{}
	seenIDs := map[int64]bool{}
	query := url.Values{"perPage": {"100"}}
	for {
		path := "/api/event-sessions/" + url.PathEscape(source.sessionID) + "/transcript/list?" + query.Encode()
		resp, data, err := m.get(ctx, op, path, "application/json", maxJSON)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, statusError{op, resp.StatusCode}
		}
		var page struct {
			Data struct {
				Items          []transcript `json:"items"`
				NextPageCursor string       `json:"nextPageCursor"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &page) != nil || page.Data.Items == nil {
			return nil, errors.New("invalid MTS Link transcript list")
		}
		for _, item := range page.Data.Items {
			if item.ID <= 0 || item.CreatedAt.IsZero() {
				return nil, errors.New("MTS Link transcript lacks an ID or creation time")
			}
			if !item.CreatedAt.Before(createdFrom) && !seenIDs[item.ID] {
				seenIDs[item.ID] = true
				all = append(all, item)
			}
		}
		cursor := page.Data.NextPageCursor
		if cursor == "" {
			return all, nil
		}
		if cursors[cursor] {
			return nil, errors.New("MTS Link returned a repeated transcript cursor")
		}
		cursors[cursor] = true
		query.Set("cursor", cursor)
	}
}

func (m *mtsClient) download(ctx context.Context, item transcript) ([]byte, string, error) {
	op := "download MTS Link transcript"
	path := "/api/transcript/" + strconv.FormatInt(item.ID, 10) + "/download"
	resp, data, err := m.get(ctx, op, path, "text/plain, application/octet-stream", maxTranscript)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", statusError{op, resp.StatusCode}
	}
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if len(data) == 0 || (contentType != "text/plain" && contentType != "application/octet-stream" && contentType != "text/markdown") {
		return nil, "", errors.New("MTS Link returned an empty or unexpected transcript download")
	}
	_, disposition, _ := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	extension := strings.ToLower(filepath.Ext(disposition["filename"]))
	if extension != ".txt" && extension != ".md" {
		extension = ".txt"
	}
	return data, extension, nil
}
