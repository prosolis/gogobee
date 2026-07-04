package bot

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// masAuth performs MAS (Matrix Authentication Service) OAuth 2.0 device-grant
// authentication for the bot and keeps a valid access token available.
//
// Why this exists: MAS replaces Matrix's legacy password login with OAuth2. A
// bot can't do the interactive authorization-code flow, so we use the OAuth 2.0
// Device Authorization Grant (RFC 8628): on first run the bot prints a URL +
// user code, a human approves the login as the bot's user ONCE in a browser,
// and MAS returns an access token + refresh token. Thereafter the bot refreshes
// silently forever — no password, no expiry surprises, and the bot stays a
// normal Matrix user so /sync keeps working (unlike an appservice user, which
// Synapse forbids from /sync).
//
// The refresh token is rotated by MAS on every refresh, so we persist the new
// one each time.
type masAuth struct {
	http      *http.Client
	storePath string
	scope     string

	// discovered OAuth endpoints
	deviceEndpoint       string
	tokenEndpoint        string
	registrationEndpoint string

	mu           sync.RWMutex
	clientID     string
	deviceID     string
	accessToken  string
	refreshToken string
	expiresAt    time.Time
}

// masStore is the on-disk persisted state (data/mas_auth.json).
type masStore struct {
	ClientID     string    `json:"client_id"`
	DeviceID     string    `json:"device_id"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

const masScopeAPI = "urn:matrix:org.matrix.msc2967.client:api:*"

func newMASAuth(dataDir string) *masAuth {
	return &masAuth{
		http:      &http.Client{Timeout: 30 * time.Second},
		storePath: filepath.Join(dataDir, "mas_auth.json"),
	}
}

func (m *masAuth) load() {
	data, err := os.ReadFile(m.storePath)
	if err != nil {
		return // fresh install
	}
	var s masStore
	if err := json.Unmarshal(data, &s); err != nil {
		slog.Warn("mas_auth: corrupt store, ignoring", "err", err)
		return
	}
	m.clientID = s.ClientID
	m.deviceID = s.DeviceID
	m.accessToken = s.AccessToken
	m.refreshToken = s.RefreshToken
	m.expiresAt = s.ExpiresAt
}

func (m *masAuth) save() {
	m.mu.RLock()
	s := masStore{
		ClientID:     m.clientID,
		DeviceID:     m.deviceID,
		AccessToken:  m.accessToken,
		RefreshToken: m.refreshToken,
		ExpiresAt:    m.expiresAt,
	}
	m.mu.RUnlock()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		slog.Error("mas_auth: marshal failed", "err", err)
		return
	}
	if err := os.WriteFile(m.storePath, data, 0o600); err != nil {
		slog.Error("mas_auth: write failed", "err", err)
	}
}

func (m *masAuth) token() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accessToken
}

// discover resolves the MAS OAuth endpoints from the homeserver's well-known
// document and the issuer's OIDC discovery document.
func (m *masAuth) discover(homeserver string) error {
	var wk struct {
		Auth struct {
			Issuer string `json:"issuer"`
		} `json:"org.matrix.msc2965.authentication"`
	}
	if err := m.getJSON(strings.TrimRight(homeserver, "/")+"/.well-known/matrix/client", &wk); err != nil {
		return fmt.Errorf("fetch well-known: %w", err)
	}
	if wk.Auth.Issuer == "" {
		return fmt.Errorf("homeserver does not advertise a MAS issuer (msc2965) — is MAS enabled?")
	}
	var oidc struct {
		DeviceEndpoint       string `json:"device_authorization_endpoint"`
		TokenEndpoint        string `json:"token_endpoint"`
		RegistrationEndpoint string `json:"registration_endpoint"`
	}
	if err := m.getJSON(strings.TrimRight(wk.Auth.Issuer, "/")+"/.well-known/openid-configuration", &oidc); err != nil {
		return fmt.Errorf("fetch OIDC discovery: %w", err)
	}
	if oidc.DeviceEndpoint == "" || oidc.TokenEndpoint == "" {
		return fmt.Errorf("MAS issuer %q does not advertise a device/token endpoint", wk.Auth.Issuer)
	}
	m.deviceEndpoint = oidc.DeviceEndpoint
	m.tokenEndpoint = oidc.TokenEndpoint
	m.registrationEndpoint = oidc.RegistrationEndpoint
	slog.Info("mas_auth: discovered endpoints", "issuer", wk.Auth.Issuer)
	return nil
}

// ensureClient registers a public OAuth client via dynamic registration if we
// don't already have a client_id persisted.
func (m *masAuth) ensureClient(displayName string) error {
	if m.clientID != "" {
		return nil
	}
	if m.registrationEndpoint == "" {
		return fmt.Errorf("no registration endpoint discovered")
	}
	body := map[string]any{
		"client_name":                displayName + " (bot)",
		"application_type":           "native",
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token"},
		"response_types":             []string{},
		"client_uri":                 "https://github.com/prosolis/gogobee",
	}
	raw, _ := json.Marshal(body)
	resp, err := m.http.Post(m.registrationEndpoint, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		return fmt.Errorf("client registration request: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("client registration failed (HTTP %d): %s", resp.StatusCode, string(rb))
	}
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(rb, &out); err != nil || out.ClientID == "" {
		return fmt.Errorf("client registration: no client_id in response: %s", string(rb))
	}
	m.mu.Lock()
	m.clientID = out.ClientID
	m.mu.Unlock()
	m.save()
	slog.Info("mas_auth: registered OAuth client", "client_id", out.ClientID)
	return nil
}

// deviceFlow runs the interactive device-authorization grant. It blocks until
// the user approves the login (or the code expires).
func (m *masAuth) deviceFlow(ctx context.Context) error {
	if m.deviceID == "" {
		m.deviceID = randomDeviceID()
	}
	scope := masScopeAPI + " urn:matrix:org.matrix.msc2967.client:device:" + m.deviceID

	form := url.Values{"client_id": {m.clientID}, "scope": {scope}}
	var da struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	if err := m.postForm(m.deviceEndpoint, form, &da); err != nil {
		return fmt.Errorf("device authorization request: %w", err)
	}

	// Surface the approval prompt prominently — the operator must act on it.
	uri := da.VerificationURI
	if da.VerificationURIComplete != "" {
		uri = da.VerificationURIComplete
	}
	banner := fmt.Sprintf(`

================= TwinBee needs you to authorize its login =================
  Open this URL in a browser (logged in as the bot's Matrix user):
      %s
  and enter code:  %s
  (expires in %d minutes)
============================================================================

`, uri, da.UserCode, da.ExpiresIn/60)
	fmt.Print(banner)
	slog.Warn("mas_auth: device authorization required", "verification_uri", da.VerificationURI, "user_code", da.UserCode, "expires_in_s", da.ExpiresIn)

	interval := da.Interval
	if interval <= 0 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(da.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
		pending, err := m.pollToken(url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {da.DeviceCode},
			"client_id":   {m.clientID},
		})
		if err != nil {
			return err
		}
		if !pending {
			slog.Info("mas_auth: device authorization approved", "device_id", m.deviceID)
			return nil
		}
	}
	return fmt.Errorf("device authorization timed out (not approved within %d minutes)", da.ExpiresIn/60)
}

// pollToken hits the token endpoint. Returns pending=true if the user hasn't
// approved yet (keep polling); on success it stores the tokens.
func (m *masAuth) pollToken(form url.Values) (pending bool, err error) {
	resp, err := m.http.PostForm(m.tokenEndpoint, form)
	if err != nil {
		return false, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 200 {
		return false, m.storeTokenResponse(rb)
	}
	var oerr struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rb, &oerr)
	switch oerr.Error {
	case "authorization_pending":
		return true, nil
	case "slow_down":
		return true, nil
	default:
		return false, fmt.Errorf("token endpoint error (HTTP %d): %s", resp.StatusCode, string(rb))
	}
}

// refresh exchanges the stored refresh token for a fresh access token. MAS
// rotates the refresh token, so we persist the new one.
func (m *masAuth) refresh() error {
	m.mu.RLock()
	rt, cid := m.refreshToken, m.clientID
	m.mu.RUnlock()
	if rt == "" {
		return fmt.Errorf("no refresh token")
	}
	resp, err := m.http.PostForm(m.tokenEndpoint, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {rt},
		"client_id":     {cid},
	})
	if err != nil {
		return fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("refresh failed (HTTP %d): %s", resp.StatusCode, string(rb))
	}
	return m.storeTokenResponse(rb)
}

func (m *masAuth) storeTokenResponse(rb []byte) error {
	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(rb, &tr); err != nil {
		return fmt.Errorf("parse token response: %w", err)
	}
	if tr.AccessToken == "" {
		return fmt.Errorf("token response missing access_token: %s", string(rb))
	}
	m.mu.Lock()
	m.accessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		m.refreshToken = tr.RefreshToken
	}
	ttl := tr.ExpiresIn
	if ttl <= 0 {
		ttl = 300
	}
	m.expiresAt = time.Now().Add(time.Duration(ttl) * time.Second)
	m.mu.Unlock()
	m.save()
	return nil
}

func (m *masAuth) expiry() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.expiresAt
}

// getJSON GETs a URL and decodes JSON.
func (m *masAuth) getJSON(u string, out any) error {
	resp, err := m.http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s: HTTP %d: %s", u, resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// postForm POSTs a form and decodes a JSON success body (200).
func (m *masAuth) postForm(u string, form url.Values, out any) error {
	resp, err := m.http.PostForm(u, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("POST %s: HTTP %d: %s", u, resp.StatusCode, string(rb))
	}
	return json.Unmarshal(rb, out)
}

// randomDeviceID mirrors mautrix's device-id style: 10 uppercase letters.
func randomDeviceID() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		// rand.Read essentially never fails; fall back to a fixed prefix.
		return "GOGOBEEBOT"
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return "GB" + string(b[:8])
}
