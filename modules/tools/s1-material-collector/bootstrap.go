package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validID(id string) bool { return idPattern.MatchString(id) }

func installSecret(ctx context.Context, kubeContext, name string, data map[string][]byte) error {
	secrets := struct {
		APIVersion string            `json:"apiVersion"`
		Kind       string            `json:"kind"`
		Metadata   map[string]string `json:"metadata"`
		Data       map[string][]byte `json:"data"`
	}{"v1", "Secret", map[string]string{"name": name, "namespace": prodNamespace}, data}
	input, err := json.Marshal(secrets)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "kubectl", "--context", kubeContext, "-n", prodNamespace, "apply",
		"--server-side", "--field-manager="+collectorName+"-bootstrap", "--force-conflicts", "-f", "-")
	cmd.Stdin = bytes.NewReader(input)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl could not install Secret %s: %w", name, err)
	}
	slog.Info("installed collector Secret", "name", name, "namespace", prodNamespace)
	return nil
}

func configureMTS(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("configure-mts", flag.ContinueOnError)
	kubeContext := flags.String("context", "", "kubectl context of the System1 production cluster")
	source := flags.String("http-file", "a.http", "local HTTP Client file with session IDs and refresh response")
	if err := flags.Parse(args); err != nil {
		return flagResult(err)
	}
	if *kubeContext == "" || flags.NArg() != 0 {
		return errors.New("configure-mts requires --context")
	}
	data, err := mtsBootstrapData(*source)
	if err != nil {
		return err
	}
	return installSecret(ctx, *kubeContext, mtsSecret, data)
}

func mtsBootstrapData(source string) (map[string][]byte, error) {
	file, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	ids := map[string]string{}
	headers := make(http.Header)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "@") {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				ids[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
		key, value, _ := strings.Cut(line, ":")
		if strings.EqualFold(key, "Set-Cookie") {
			headers.Add("Set-Cookie", strings.TrimSpace(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	response := http.Response{Header: headers}
	var session mtsSession
	for _, cookie := range response.Cookies() {
		switch cookie.Name {
		case "access":
			session.access, session.accessExpires = cookie.Value, cookieExpiry(cookie)
		case "refresh":
			session.refresh, session.refreshExpires = cookie.Value, cookieExpiry(cookie)
		}
	}
	banya, system1 := ids["@banyaEventSessionId"], ids["@system1EventSessionId"]
	if session.refresh == "" || session.refreshExpires.Before(time.Now()) && !session.refreshExpires.IsZero() {
		return nil, errors.New("capture a fresh BrowserRefresh response with a valid refresh cookie")
	}
	if !validID(banya) || !validID(system1) || banya == system1 {
		return nil, errors.New("expected different banyaEventSessionId and system1EventSessionId values in the local HTTP file")
	}
	data := sessionData(session)
	data["banya_session_id"] = []byte(banya)
	data["system1_session_id"] = []byte(system1)
	return data, nil
}

func configureDrive(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("configure-drive", flag.ContinueOnError)
	kubeContext := flags.String("context", "", "kubectl context of the System1 production cluster")
	clientFile := flags.String("client-json", "", "local Desktop OAuth client JSON downloaded from Google Cloud")
	project := flags.String("picker-project", "", "Google Cloud project number for Picker")
	parent := flags.String("parent", "", "expected Google Drive parent folder ID")
	if err := flags.Parse(args); err != nil {
		return flagResult(err)
	}
	pickerKey := os.Getenv("GOOGLE_PICKER_API_KEY")
	if flags.NArg() != 0 || *kubeContext == "" || *clientFile == "" || !validID(*project) || !validID(*parent) || pickerKey == "" {
		return errors.New("configure-drive requires --context, --client-json, --picker-project, --parent and GOOGLE_PICKER_API_KEY")
	}
	configuration, err := os.ReadFile(*clientFile)
	if err != nil {
		return err
	}
	var credentials struct {
		Installed struct {
			ID     string `json:"client_id"`
			Secret string `json:"client_secret"`
		}
	}
	if json.Unmarshal(configuration, &credentials) != nil || credentials.Installed.ID == "" || credentials.Installed.Secret == "" {
		return errors.New("invalid Google Desktop OAuth client JSON")
	}
	config := googleConfig(credentials.Installed.ID, credentials.Installed.Secret)
	refresh, err := authorizeDrive(ctx, config, pickerKey, *project, *parent)
	if err != nil {
		return err
	}
	return installSecret(ctx, *kubeContext, driveSecret, map[string][]byte{
		"client_id": []byte(config.ClientID), "client_secret": []byte(config.ClientSecret),
		"refresh_token": []byte(refresh), "parent_folder_id": []byte(*parent),
	})
}

func flagResult(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func authorizeDrive(ctx context.Context, config oauth2.Config, pickerKey, project, parent string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: requestTimeout})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	state, verifier := rand.Text(), oauth2.GenerateVerifier()
	config.RedirectURL = "http://" + listener.Addr().String() + "/callback"

	var mu sync.Mutex
	var refresh string
	result := make(chan string, 1)
	callbackUsed := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "origin")
		if r.URL.Query().Get("state") != state || r.URL.Query().Get("code") == "" {
			http.Error(w, "Google authorization was cancelled or invalid.", http.StatusBadRequest)
			return
		}
		mu.Lock()
		if callbackUsed {
			mu.Unlock()
			http.Error(w, "This authorization link was already used.", http.StatusConflict)
			return
		}
		callbackUsed = true
		mu.Unlock()
		token, err := config.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
		if err != nil || token.RefreshToken == "" {
			http.Error(w, "Google did not issue a refresh token. Restart setup.", http.StatusBadGateway)
			return
		}
		mu.Lock()
		refresh = token.RefreshToken
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := folderPicker.Execute(w, struct{ Key, Project, AccessToken, State string }{pickerKey, project, token.AccessToken, state}); err != nil {
			slog.Error("render Google folder picker")
		}
	})
	mux.HandleFunc("POST /folder", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var selection struct{ ID, State string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&selection) != nil || selection.State != state || selection.ID != parent {
			http.Error(w, "Select the configured System1 folder.", http.StatusBadRequest)
			return
		}
		mu.Lock()
		saved := refresh
		mu.Unlock()
		if saved == "" {
			http.Error(w, "Complete Google OAuth first.", http.StatusForbidden)
			return
		}
		client := newDriveClient(ctx, config, &oauth2.Token{RefreshToken: saved})
		if err := client.checkParent(ctx, parent); err != nil {
			http.Error(w, "The app cannot write to the selected folder.", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		select {
		case result <- saved:
		default:
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer func() {
		closeContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(closeContext)
		listener.Close()
	}()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	fmt.Println(config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", "consent")))
	select {
	case token := <-result:
		return token, nil
	case err := <-serveErr:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
