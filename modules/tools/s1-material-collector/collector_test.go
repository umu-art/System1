package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestDriveUsesGoogleRefreshToken(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			refreshes.Add(1)
			if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "stored-refresh" {
				t.Error("Google did not receive the expected refresh token request")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"access_token":"short-lived","token_type":"Bearer","expires_in":3600}`)
		case "/files/system1-folder":
			if r.Header.Get("Authorization") != "Bearer short-lived" || r.URL.Query().Get("supportsAllDrives") != "true" {
				t.Error("Drive did not receive the refreshed access token")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"id":"system1-folder","mimeType":"application/vnd.google-apps.folder","capabilities":{"canAddChildren":true}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	ctx := context.Background()
	config := googleConfig("test-client", "test-secret")
	config.Endpoint.TokenURL = server.URL + "/token"
	for range 2 {
		client := newDriveClient(ctx, config, &oauth2.Token{RefreshToken: "stored-refresh"})
		client.api = server.URL
		if err := client.checkParent(ctx, "system1-folder"); err != nil {
			t.Fatal(err)
		}
	}
	if refreshes.Load() != 2 {
		t.Fatalf("expected each run to refresh access independently, got %d refreshes", refreshes.Load())
	}
}

func TestMTSListRefreshAndPagination(t *testing.T) {
	ctx := context.Background()
	var saved atomic.Bool
	const nextCursor = "cursor+/?="
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accountUcaas/AccountUcaas.BrowserRefresh":
			cookie, err := r.Cookie("refresh")
			if r.Method != http.MethodPost || err != nil || cookie.Value != "old-refresh" {
				t.Error("refresh request did not use the current refresh cookie")
			}
			http.SetCookie(w, &http.Cookie{Name: "access", Value: "new-access", Expires: time.Now().Add(time.Hour)})
			http.SetCookie(w, &http.Cookie{Name: "refresh", Value: "new-refresh", Expires: time.Now().Add(24 * time.Hour)})
		case "/api/event-sessions/session/transcript/list":
			access, err := r.Cookie("access")
			if err != nil || access.Value == "old-access" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if !saved.Load() || access.Value != "new-access" || r.URL.Query().Get("perPage") != "100" {
				t.Error("a transcript request used unpersisted or invalid credentials")
			}
			if cursor := r.URL.Query().Get("cursor"); cursor == "" {
				fmt.Fprintf(w, `{"data":{"items":[{"transcriptId":123,"createdAt":"2026-11-20T12:00:00Z"},{"transcriptId":122,"createdAt":"2026-11-10T20:59:59Z"}],"nextPageCursor":%q}}`, nextCursor)
			} else if cursor == nextCursor {
				fmt.Fprint(w, `{"data":{"items":[{"transcriptId":124,"createdAt":"2026-11-10T21:00:00Z"}],"nextPageCursor":null}}`)
			} else {
				t.Errorf("unexpected cursor %q", cursor)
				w.WriteHeader(http.StatusBadRequest)
			}
		default:
			t.Errorf("unexpected MTS path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := newMTSClient(mtsSession{access: "old-access", refresh: "old-refresh"}, func(_ context.Context, session mtsSession) error {
		if session.refresh != "new-refresh" || session.access != "new-access" || session.refreshExpires.IsZero() {
			t.Fatal("refresh produced the wrong session")
		}
		saved.Store(true)
		return nil
	})
	client.gateway = server.URL

	items, err := client.list(ctx, room{name: "Баня", sessionID: "session"}, exportFrom)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != 123 || items[1].ID != 124 {
		t.Fatalf("unexpected transcript list: %+v", items)
	}
}

func TestCollectorRecoversUploadAfterStateWriteFailure(t *testing.T) {
	ctx := context.Background()
	transcriptData := []byte("Пример расшифровки")
	item := transcript{ID: 124, CreatedAt: time.Date(2026, 11, 20, 16, 0, 0, 0, time.UTC)}
	const key = "session/124"
	var mu sync.Mutex
	var persisted []byte
	var uploaded driveFile
	var uploads int
	var downloads atomic.Int32
	rejectedWrite := false
	drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/files" && r.Method == http.MethodGet:
			fmt.Fprint(w, `{"files":[]}`)
		case r.URL.Path == "/files/generateIds":
			fmt.Fprint(w, `{"ids":["reserved-id"]}`)
		case r.URL.Path == "/files/state-id" && r.Method == http.MethodPatch:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if uploaded.ID != "" && !bytes.Contains(body, []byte("uploaded_at")) {
				t.Error("expected the uploaded state to include a timestamp")
			}
			if uploaded.ID != "" && !rejectedWrite {
				rejectedWrite = true
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			persisted = body
			fmt.Fprint(w, `{"id":"state-id"}`)
		case r.URL.Path == "/files" && r.Method == http.MethodPost:
			if len(persisted) == 0 || !bytes.Contains(persisted, []byte("reserved-id")) {
				t.Error("upload started before reservation was persisted")
			}
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "multipart/related" {
				t.Error("invalid Drive upload content type")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			part, err := reader.NextPart()
			if err != nil || json.NewDecoder(part).Decode(&uploaded) != nil {
				t.Error("invalid upload metadata")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			part, err = reader.NextPart()
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			data, err := io.ReadAll(part)
			if err != nil || !bytes.Equal(data, transcriptData) {
				t.Error("Drive received an unexpected transcript")
			}
			uploaded.Size = int64(len(data))
			uploads++
			json.NewEncoder(w).Encode(uploaded)
		case r.URL.Path == "/files/reserved-id" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(uploaded)
		default:
			t.Errorf("unexpected Drive request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer drive.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		w.Write(transcriptData)
	}))
	defer source.Close()

	client := &driveClient{http: drive.Client(), api: drive.URL, upload: drive.URL}
	mts := newMTSClient(mtsSession{access: "test-cookie"}, nil)
	mts.gateway = source.URL
	c := collector{drive: client, mts: mts, stateID: "state-id", state: exportState{Version: 1, Entries: map[string]exportEntry{}}}
	if _, err := c.storeTranscript(ctx, "room-folder", key, item); err == nil {
		t.Fatal("expected a failure while recording the upload")
	}

	var reloaded exportState
	mu.Lock()
	err := json.Unmarshal(persisted, &reloaded)
	mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Entries[key].DriveID != "reserved-id" || !reloaded.Entries[key].UploadedAt.IsZero() {
		t.Fatalf("unexpected state after interruption: %+v", reloaded)
	}
	second := collector{drive: client, mts: mts, stateID: "state-id", state: reloaded}
	outcome, err := second.storeTranscript(ctx, "room-folder", key, item)
	if err != nil || outcome != "recovered" {
		t.Fatalf("got %q, %v, want a recovered upload", outcome, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if uploads != 1 || downloads.Load() != 1 || second.state.Entries[key].UploadedAt.IsZero() {
		t.Fatalf("expected exactly one transfer and completed state, got %d uploads and %d downloads", uploads, downloads.Load())
	}
}
