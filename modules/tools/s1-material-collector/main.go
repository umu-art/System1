package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/oauth2"
)

const credentialsExitCode = 78

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("collector failed", "error", err)
		var auth credentialsError
		if errors.As(err, &auth) {
			os.Exit(credentialsExitCode)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) != 0 {
		switch args[0] {
		case "configure-mts":
			return configureMTS(ctx, args[1:])
		case "configure-drive":
			return configureDrive(ctx, args[1:])
		case "-h", "--help":
			fmt.Println("s1-material-collector [configure-mts | configure-drive]")
			return nil
		default:
			return errors.New("unknown command, use --help")
		}
	}
	return runCollector(ctx)
}

func runCollector(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, maxRun)
	defer cancel()
	cluster, err := clusterClient()
	if err != nil {
		return err
	}
	holder := os.Getenv("POD_UID")
	if err := cluster.lock(ctx, holder); err != nil {
		return fmt.Errorf("wait for exclusive collector run: %w", err)
	}
	defer func() {
		releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := cluster.unlock(releaseContext, holder); err != nil {
			slog.Warn("collector Lease release failed", "error", err)
		}
	}()

	mts, err := cluster.secret(ctx, mtsSecret)
	if err != nil {
		return err
	}
	google, err := cluster.secret(ctx, driveSecret)
	if err != nil {
		return err
	}

	rooms := []room{{"Баня", string(mts.Data["banya_session_id"])}, {"System1", string(mts.Data["system1_session_id"])}}
	if !validID(rooms[0].sessionID) || !validID(rooms[1].sessionID) || rooms[0].sessionID == rooms[1].sessionID {
		return credentialsError{"KUBERNETES_SECRET: expected two distinct MTS Link session IDs"}
	}

	session := mtsSession{access: string(mts.Data["access"]), refresh: string(mts.Data["refresh"])}
	session.accessExpires = expiryFromSecret(string(mts.Data["access_expires_at"]), session.access)
	session.refreshExpires = expiryFromSecret(string(mts.Data["refresh_expires_at"]), session.refresh)

	parent := string(google.Data["parent_folder_id"])
	client := googleConfig(string(google.Data["client_id"]), string(google.Data["client_secret"]))
	token := &oauth2.Token{RefreshToken: string(google.Data["refresh_token"])}
	if !validID(parent) || client.ClientID == "" || client.ClientSecret == "" || token.RefreshToken == "" {
		return credentialsError{"KUBERNETES_SECRET: incomplete Google Drive configuration"}
	}

	job := collector{
		drive: newDriveClient(ctx, client, token),
		mts: newMTSClient(session, func(ctx context.Context, next mtsSession) error {
			return cluster.saveMTS(ctx, &mts, next)
		}),
	}
	return job.sync(ctx, parent, rooms)
}

func expiryFromSecret(raw, token string) time.Time {
	expiry, err := time.Parse(time.RFC3339, raw)
	if err == nil {
		return expiry
	}
	return jwtExpiry(token)
}
