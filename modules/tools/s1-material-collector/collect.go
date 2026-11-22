package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

const (
	exportFolder  = "MTS Link Export"
	stateFile     = "state.json"
	transcriptKey = "s1TranscriptKey"
)

var moscow = time.FixedZone("Europe/Moscow", 3*60*60)
var exportFrom = time.Date(2026, time.November, 11, 0, 0, 0, 0, moscow)

type exportEntry struct {
	DriveID    string    `json:"drive_id"`
	UploadedAt time.Time `json:"uploaded_at,omitzero"`
}

type exportState struct {
	Version int                    `json:"version"`
	Entries map[string]exportEntry `json:"entries"`
}

type collector struct {
	mts     *mtsClient
	drive   *driveClient
	state   exportState
	stateID string
}

func (c *collector) openState(ctx context.Context, root string) error {
	file, err := c.drive.find(ctx, root, "name = "+driveLiteral(stateFile))
	if err != nil {
		return err
	}
	if file.ID != "" {
		c.stateID = file.ID
		data, err := c.drive.readState(ctx, file.ID)
		if err != nil {
			return err
		}
		if json.Unmarshal(data, &c.state) != nil || c.state.Version != 1 || c.state.Entries == nil {
			return errors.New("Drive state.json has an invalid or unsupported format")
		}
		return nil
	}
	c.state = exportState{Version: 1, Entries: map[string]exportEntry{}}
	id, err := c.drive.nextID(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(c.state)
	if err != nil {
		return err
	}
	expected := driveFile{ID: id, Name: stateFile, MIME: "application/json", Parents: []string{root}}
	file, err = c.drive.create(ctx, expected, data)
	if err != nil {
		return err
	}
	if err := verifyDriveFile(file, expected, int64(len(data))); err != nil {
		return err
	}
	c.stateID = file.ID
	return nil
}

func (c *collector) saveEntry(ctx context.Context, key string, entry exportEntry) error {
	old, existed := c.state.Entries[key]
	c.state.Entries[key] = entry
	data, err := json.Marshal(c.state)
	if err == nil {
		err = c.drive.writeState(ctx, c.stateID, data)
	}
	if err != nil {
		if existed {
			c.state.Entries[key] = old
		} else {
			delete(c.state.Entries, key)
		}
	}
	return err
}

func (c *collector) storeTranscript(ctx context.Context, folder, key string, item transcript) (string, error) {
	entry := c.state.Entries[key]
	if !entry.UploadedAt.IsZero() {
		return "existing", nil
	}
	if entry.DriveID != "" {
		file, err := c.drive.file(ctx, entry.DriveID)
		if err == nil {
			expected := driveFile{ID: entry.DriveID, MIME: "text/plain", Parents: []string{folder}, AppProperties: map[string]string{transcriptKey: key}}
			if err := verifyDriveFile(file, expected, -1); err != nil {
				return "", err
			}
			entry.UploadedAt = time.Now().UTC()
			return "recovered", c.saveEntry(ctx, key, entry)
		}
		if !hasStatus(err, http.StatusNotFound) {
			return "", err
		}
	} else {
		filter := "appProperties has { key=" + driveLiteral(transcriptKey) + " and value=" + driveLiteral(key) + " }"
		file, err := c.drive.find(ctx, folder, filter)
		if err != nil {
			return "", err
		}
		if file.ID != "" {
			entry = exportEntry{DriveID: file.ID, UploadedAt: time.Now().UTC()}
			return "recovered", c.saveEntry(ctx, key, entry)
		}
	}

	data, extension, err := c.mts.download(ctx, item)
	if hasStatus(err, http.StatusAccepted) || hasStatus(err, http.StatusNoContent) ||
		(hasStatus(err, http.StatusNotFound) && time.Since(item.CreatedAt) < 2*time.Hour) {
		return "pending", nil
	}
	if err != nil {
		return "", err
	}
	if entry.DriveID == "" {
		entry.DriveID, err = c.drive.nextID(ctx)
		if err != nil {
			return "", err
		}
		if err := c.saveEntry(ctx, key, entry); err != nil {
			return "", err
		}
	}
	name := item.CreatedAt.In(moscow).Format("2006-01-02_15-04") + "_" + strconv.FormatInt(item.ID, 10) + extension
	expected := driveFile{
		ID: entry.DriveID, Name: name, MIME: "text/plain", Parents: []string{folder},
		AppProperties: map[string]string{transcriptKey: key},
	}
	file, err := c.drive.create(ctx, expected, data)
	if err != nil {
		return "", err
	}
	if err := verifyDriveFile(file, expected, int64(len(data))); err != nil {
		return "", err
	}
	entry.UploadedAt = time.Now().UTC()
	return "uploaded", c.saveEntry(ctx, key, entry)
}

func (c *collector) syncRoom(ctx context.Context, root string, source room) error {
	items, err := c.mts.list(ctx, source, exportFrom)
	if err != nil {
		return err
	}
	folder, err := c.drive.folder(ctx, root, source.name)
	if err != nil {
		return err
	}
	stats := map[string]int{}
	var failures []error
	for _, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := source.sessionID + "/" + strconv.FormatInt(item.ID, 10)
		outcome, err := c.storeTranscript(ctx, folder, key, item)
		if err != nil {
			failures = append(failures, fmt.Errorf("transcript %d: %w", item.ID, err))
			var auth credentialsError
			if errors.As(err, &auth) {
				break
			}
			continue
		}
		stats[outcome]++
	}
	slog.Info("room export", "room", source.name, "listed", len(items), "uploaded", stats["uploaded"], "recovered", stats["recovered"], "existing", stats["existing"], "pending", stats["pending"], "failed", len(failures))
	return errors.Join(failures...)
}

func (c *collector) sync(ctx context.Context, parent string, rooms []room) error {
	if err := c.mts.prepare(ctx); err != nil {
		return err
	}
	if err := c.drive.checkParent(ctx, parent); err != nil {
		return err
	}
	root, err := c.drive.folder(ctx, parent, exportFolder)
	if err != nil {
		return err
	}
	if err := c.openState(ctx, root); err != nil {
		return err
	}
	var failures []error
	for _, source := range rooms {
		if err := c.syncRoom(ctx, root, source); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", source.name, err))
		}
	}
	return errors.Join(failures...)
}
