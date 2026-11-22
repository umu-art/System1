package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	driveAPI       = "https://www.googleapis.com/drive/v3"
	driveUploadAPI = "https://www.googleapis.com/upload/drive/v3"
	driveScope     = "https://www.googleapis.com/auth/drive.file"
	folderMIME     = "application/vnd.google-apps.folder"
	fileFields     = "id,name,mimeType,parents,appProperties,size,trashed,capabilities(canAddChildren)"
)

type driveClient struct {
	http   *http.Client
	api    string
	upload string
}

type driveFile struct {
	ID            string            `json:"id,omitempty"`
	Name          string            `json:"name,omitempty"`
	MIME          string            `json:"mimeType,omitempty"`
	Parents       []string          `json:"parents,omitempty"`
	AppProperties map[string]string `json:"appProperties,omitempty"`
	Size          int64             `json:"size,string,omitempty"`
	Trashed       bool              `json:"trashed,omitempty"`
	Capabilities  *struct {
		CanAddChildren bool `json:"canAddChildren"`
	} `json:"capabilities,omitempty"`
}

func googleConfig(clientID, clientSecret string) oauth2.Config {
	return oauth2.Config{
		ClientID: clientID, ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{AuthURL: googleAuthURL, TokenURL: googleTokenURL, AuthStyle: oauth2.AuthStyleInParams},
		Scopes:   []string{driveScope},
	}
}

type driveTokenSource struct{ oauth2.TokenSource }

func (s driveTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.TokenSource.Token()
	var rejected *oauth2.RetrieveError
	if errors.As(err, &rejected) && (rejected.ErrorCode == "invalid_grant" || rejected.ErrorCode == "invalid_client" || rejected.ErrorCode == "unauthorized_client") {
		return nil, credentialsError{"GOOGLE_DRIVE_AUTH: Google rejected the OAuth credentials"}
	}
	return token, err
}

func newDriveClient(ctx context.Context, config oauth2.Config, token *oauth2.Token) *driveClient {
	httpClient := &http.Client{Timeout: requestTimeout}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	client := oauth2.NewClient(ctx, driveTokenSource{config.TokenSource(ctx, token)})
	client.Timeout = requestTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &driveClient{http: client, api: driveAPI, upload: driveUploadAPI}
}

func (d *driveClient) call(ctx context.Context, operation, method, endpoint, contentType string, payload []byte, limit int64) ([]byte, error) {
	headers := make(http.Header)
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	resp, data, err := request(ctx, d.http, operation, method, endpoint, headers, payload, 3, limit)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, credentialsError{"GOOGLE_DRIVE_AUTH: Google Drive rejected the access token"}
	case http.StatusForbidden:
		var failure struct {
			Error struct {
				Errors []struct{ Reason string }
			}
		}
		if json.Unmarshal(data, &failure) == nil {
			for _, issue := range failure.Error.Errors {
				switch issue.Reason {
				case "rateLimitExceeded", "userRateLimitExceeded", "storageQuotaExceeded", "teamDriveFileLimitExceeded":
					return nil, statusError{operation + " (" + issue.Reason + ")", resp.StatusCode}
				}
			}
		}
		return nil, credentialsError{"GOOGLE_DRIVE_AUTH: access to the target Drive files was denied"}
	default:
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, statusError{operation, resp.StatusCode}
		}
	}
	return data, nil
}

func decodeFile(data []byte) (driveFile, error) {
	var file driveFile
	if json.Unmarshal(data, &file) != nil || file.ID == "" {
		return file, errors.New("invalid Google Drive file metadata")
	}
	return file, nil
}

func fileURL(base, id string, query url.Values) string {
	query.Set("supportsAllDrives", "true")
	return base + "/files/" + url.PathEscape(id) + "?" + query.Encode()
}

func (d *driveClient) file(ctx context.Context, id string) (driveFile, error) {
	endpoint := fileURL(d.api, id, url.Values{"fields": {fileFields}})
	data, err := d.call(ctx, "read Drive file metadata", http.MethodGet, endpoint, "", nil, maxJSON)
	if err != nil {
		return driveFile{}, err
	}
	return decodeFile(data)
}

func (d *driveClient) checkParent(ctx context.Context, id string) error {
	file, err := d.file(ctx, id)
	if hasStatus(err, http.StatusNotFound) {
		return credentialsError{"GOOGLE_DRIVE_AUTH: selected parent folder is missing or has not been granted to the app"}
	}
	if err != nil {
		return err
	}
	if file.MIME != folderMIME || file.Trashed || file.Capabilities == nil || !file.Capabilities.CanAddChildren {
		return credentialsError{"GOOGLE_DRIVE_AUTH: app cannot create files in the selected folder"}
	}
	return nil
}

func driveLiteral(value string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(value) + "'"
}

func (d *driveClient) find(ctx context.Context, parent, filter string) (driveFile, error) {
	q := driveLiteral(parent) + " in parents and trashed = false and " + filter
	query := url.Values{
		"q": {q}, "fields": {"files(" + fileFields + "),nextPageToken,incompleteSearch"},
		"includeItemsFromAllDrives": {"true"}, "supportsAllDrives": {"true"}, "pageSize": {"10"},
	}
	var result driveFile
	seen := map[string]bool{}
	for {
		data, err := d.call(ctx, "find Drive files", http.MethodGet, d.api+"/files?"+query.Encode(), "", nil, maxJSON)
		if err != nil {
			return driveFile{}, err
		}
		var page struct {
			Files            []driveFile
			NextPageToken    string
			IncompleteSearch bool
		}
		if json.Unmarshal(data, &page) != nil || page.IncompleteSearch {
			return driveFile{}, errors.New("Google Drive returned an invalid or incomplete search")
		}
		for _, file := range page.Files {
			if file.ID == "" || (result.ID != "" && result.ID != file.ID) {
				return driveFile{}, errors.New("ambiguous Google Drive search result")
			}
			result = file
		}
		if page.NextPageToken == "" {
			return result, nil
		}
		if seen[page.NextPageToken] {
			return driveFile{}, errors.New("Google Drive returned a repeated page token")
		}
		seen[page.NextPageToken] = true
		query.Set("pageToken", page.NextPageToken)
	}
}

func (d *driveClient) nextID(ctx context.Context) (string, error) {
	data, err := d.call(ctx, "reserve a Drive file ID", http.MethodGet, d.api+"/files/generateIds?count=1&space=drive", "", nil, maxJSON)
	if err != nil {
		return "", err
	}
	var ids struct{ IDs []string }
	if json.Unmarshal(data, &ids) != nil || len(ids.IDs) != 1 || ids.IDs[0] == "" {
		return "", errors.New("Google Drive did not reserve a file ID")
	}
	return ids.IDs[0], nil
}

func (d *driveClient) create(ctx context.Context, file driveFile, media []byte) (driveFile, error) {
	metadata, err := json.Marshal(file)
	if err != nil {
		return driveFile{}, err
	}
	query := url.Values{"supportsAllDrives": {"true"}, "fields": {fileFields}}
	base, contentType, payload := d.api, "application/json", metadata
	if media != nil {
		base = d.upload
		query.Set("uploadType", "multipart")

		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		for _, part := range []struct {
			contentType string
			data        []byte
		}{{"application/json; charset=UTF-8", metadata}, {file.MIME, media}} {
			section, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {part.contentType}})
			if err != nil {
				return driveFile{}, err
			}
			if _, err := section.Write(part.data); err != nil {
				return driveFile{}, err
			}
		}
		if err := writer.Close(); err != nil {
			return driveFile{}, err
		}
		contentType = "multipart/related; boundary=" + writer.Boundary()
		payload = buffer.Bytes()
	}
	data, err := d.call(ctx, "create Drive file", http.MethodPost, base+"/files?"+query.Encode(), contentType, payload, maxJSON)
	if hasStatus(err, http.StatusConflict) {
		return d.file(ctx, file.ID)
	}
	if err != nil {
		return driveFile{}, err
	}
	return decodeFile(data)
}

func verifyDriveFile(result, expected driveFile, size int64) error {
	if result.ID != expected.ID || result.Trashed || result.MIME != expected.MIME || len(result.Parents) != 1 || result.Parents[0] != expected.Parents[0] {
		return errors.New("Google Drive file does not match its reserved metadata")
	}
	for key, value := range expected.AppProperties {
		if result.AppProperties[key] != value {
			return fmt.Errorf("Google Drive file is missing the expected %s property", key)
		}
	}
	if size >= 0 && size != result.Size {
		return errors.New("Google Drive file size differs from its source")
	}
	return nil
}

func (d *driveClient) folder(ctx context.Context, parent, name string) (string, error) {
	file, err := d.find(ctx, parent, "name = "+driveLiteral(name)+" and mimeType = "+driveLiteral(folderMIME))
	if err != nil || file.ID != "" {
		return file.ID, err
	}
	id, err := d.nextID(ctx)
	if err != nil {
		return "", err
	}
	expected := driveFile{ID: id, Name: name, MIME: folderMIME, Parents: []string{parent}}
	file, err = d.create(ctx, expected, nil)
	if err != nil {
		return "", err
	}
	return file.ID, verifyDriveFile(file, expected, -1)
}

func (d *driveClient) readState(ctx context.Context, id string) ([]byte, error) {
	endpoint := fileURL(d.api, id, url.Values{"alt": {"media"}})
	return d.call(ctx, "read export state", http.MethodGet, endpoint, "", nil, maxJSON)
}

func (d *driveClient) writeState(ctx context.Context, id string, data []byte) error {
	if len(data) > maxJSON {
		return errors.New("export state exceeds its size limit")
	}
	endpoint := fileURL(d.upload, id, url.Values{"uploadType": {"media"}})
	_, err := d.call(ctx, "save export state", http.MethodPatch, endpoint, "application/json", data, maxJSON)
	return err
}
