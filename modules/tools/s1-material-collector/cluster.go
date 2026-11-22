package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	collectorName  = "s1-material-collector"
	mtsSecret      = collectorName + "-mts-link"
	driveSecret    = collectorName + "-google-drive"
	prodNamespace  = "system1"
	serviceAccount = "/var/run/secrets/kubernetes.io/serviceaccount/"
	maxRun         = 12 * time.Minute
	leaseDuration  = 15 * time.Minute
)

type kubeClient struct {
	http      *http.Client
	base      string
	namespace string
}

type objectMeta struct {
	ResourceVersion string `json:"resourceVersion"`
}

type kubeSecret struct {
	Metadata objectMeta
	Data     map[string][]byte
}

type leaseSpec struct {
	HolderIdentity       string `json:"holderIdentity"`
	LeaseDurationSeconds int    `json:"leaseDurationSeconds"`
	RenewTime            string `json:"renewTime"`
}

type kubeLease struct {
	Metadata objectMeta
	Spec     leaseSpec
}

type serviceAccountTransport struct{ http.RoundTripper }

func (t serviceAccountTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := os.ReadFile(serviceAccount + "token")
	if err != nil {
		return nil, err
	}
	authenticated := req.Clone(req.Context())
	authenticated.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	return t.RoundTripper.RoundTrip(authenticated)
}

func clusterClient() (*kubeClient, error) {
	namespace, err := os.ReadFile(serviceAccount + "namespace")
	if err != nil || strings.TrimSpace(string(namespace)) != prodNamespace {
		return nil, errors.New("collector requires a service account in namespace system1")
	}
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("in-cluster Kubernetes API address is unavailable")
	}
	ca, err := os.ReadFile(serviceAccount + "ca.crt")
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid Kubernetes service account CA")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	return &kubeClient{
		http: &http.Client{
			Transport:     serviceAccountTransport{transport},
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		base: "https://" + net.JoinHostPort(host, port), namespace: prodNamespace,
	}, nil
}

func (k *kubeClient) call(ctx context.Context, operation, method, endpoint string, patch, result any) error {
	var payload []byte
	var headers http.Header
	if patch != nil {
		var err error
		payload, err = json.Marshal(patch)
		if err != nil {
			return err
		}
		headers = http.Header{"Content-Type": {"application/merge-patch+json"}}
	}
	resp, data, err := request(ctx, k.http, operation, method, k.base+endpoint, headers, payload, 1, maxJSON)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return statusError{operation, resp.StatusCode}
	}
	if result != nil {
		return json.Unmarshal(data, result)
	}
	return nil
}

func (k *kubeClient) secretPath(name string) string {
	return "/api/v1/namespaces/" + k.namespace + "/secrets/" + name
}

func (k *kubeClient) secret(ctx context.Context, name string) (kubeSecret, error) {
	var secret kubeSecret
	err := k.call(ctx, "read "+name, http.MethodGet, k.secretPath(name), nil, &secret)
	if hasStatus(err, http.StatusNotFound) || hasStatus(err, http.StatusForbidden) {
		return secret, credentialsError{"KUBERNETES_SECRET: missing or inaccessible Secret " + name}
	}
	return secret, err
}

func (k *kubeClient) saveMTS(ctx context.Context, secret *kubeSecret, tokens mtsSession) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	patch := struct {
		Metadata objectMeta        `json:"metadata"`
		Data     map[string][]byte `json:"data"`
	}{secret.Metadata, sessionData(tokens)}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var updated kubeSecret
		err = k.call(ctx, "update "+mtsSecret, http.MethodPatch, k.secretPath(mtsSecret), patch, &updated)
		if err == nil {
			*secret = updated
			return nil
		}
		if hasStatus(err, http.StatusConflict) {
			updated, readErr := k.secret(ctx, mtsSecret)
			if readErr != nil {
				return readErr
			}
			for key, value := range patch.Data {
				if !bytes.Equal(value, updated.Data[key]) {
					return errors.New("MTS Link Secret was changed during refresh; keep the newer Secret")
				}
			}
			*secret = updated
			return nil
		}
	}
	return err
}

func sessionData(session mtsSession) map[string][]byte {
	return map[string][]byte{
		"access":             []byte(session.access),
		"refresh":            []byte(session.refresh),
		"access_expires_at":  []byte(formatExpiry(session.accessExpires)),
		"refresh_expires_at": []byte(formatExpiry(session.refreshExpires)),
	}
}

func formatExpiry(expiry time.Time) string {
	if expiry.IsZero() {
		return ""
	}
	return expiry.UTC().Format(time.RFC3339)
}

func (k *kubeClient) leasePath() string {
	return "/apis/coordination.k8s.io/v1/namespaces/" + k.namespace + "/leases/" + collectorName
}

func (k *kubeClient) lock(ctx context.Context, holder string) error {
	if holder == "" {
		return errors.New("missing pod UID for the collector Lease")
	}
	path := k.leasePath()
	for {
		var lease kubeLease
		if err := k.call(ctx, "read collector Lease", http.MethodGet, path, nil, &lease); err != nil {
			return err
		}
		renewedAt, err := time.Parse(time.RFC3339Nano, lease.Spec.RenewTime)
		if lease.Spec.HolderIdentity != "" && (err != nil || lease.Spec.LeaseDurationSeconds <= 0) {
			return errors.New("collector Lease has an invalid expiration")
		}
		expiry := renewedAt.Add(time.Duration(lease.Spec.LeaseDurationSeconds) * time.Second)
		if lease.Spec.HolderIdentity == "" || lease.Spec.HolderIdentity == holder || time.Now().After(expiry) {
			patch := struct {
				Metadata objectMeta `json:"metadata"`
				Spec     leaseSpec  `json:"spec"`
			}{
				lease.Metadata,
				leaseSpec{holder, int(leaseDuration.Seconds()), time.Now().UTC().Format("2006-01-02T15:04:05.000000Z07:00")},
			}
			err := k.call(ctx, "acquire collector Lease", http.MethodPatch, path, patch, nil)
			if !hasStatus(err, http.StatusConflict) {
				return err
			}
		}
		if err := wait(ctx, 5*time.Second); err != nil {
			return err
		}
	}
}

func (k *kubeClient) unlock(ctx context.Context, holder string) error {
	var lease kubeLease
	path := k.leasePath()
	if err := k.call(ctx, "read collector Lease", http.MethodGet, path, nil, &lease); err != nil {
		return err
	}
	if lease.Spec.HolderIdentity != holder {
		return nil
	}
	patch := map[string]any{"metadata": lease.Metadata, "spec": map[string]string{"holderIdentity": ""}}
	return k.call(ctx, "release collector Lease", http.MethodPatch, path, patch, nil)
}
