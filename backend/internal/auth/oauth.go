package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

const (
	onshapeAuthURL  = "https://oauth.onshape.com/oauth/authorize"
	onshapeTokenURL = "https://oauth.onshape.com/oauth/token"
)

// OAuthConfig holds the registered OAuth2 application credentials.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
}

// oauthCfg returns a golang.org/x/oauth2 config for Onshape.
func oauthCfg(c OAuthConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.CallbackURL,
		Scopes:       []string{"OAuth2Read", "OAuth2Write"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  onshapeAuthURL,
			TokenURL: onshapeTokenURL,
		},
	}
}

// Handler provides HTTP handlers for the OAuth2 flow.
type Handler struct {
	cfg         OAuthConfig
	sessions    *Store
	oauthCfgObj *oauth2.Config
}

// NewHandler constructs an OAuth handler.
func NewHandler(cfg OAuthConfig, sessions *Store) *Handler {
	return &Handler{
		cfg:         cfg,
		sessions:    sessions,
		oauthCfgObj: oauthCfg(cfg),
	}
}

// LoginHandler redirects the user to the Onshape authorization page.
// It stores the panel's original URL in the session so it can redirect back
// after the OAuth callback completes.
//
// Expected query param: redirect (the panel URL to return to after auth).
func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.sessions.Get(r)
	if !ok {
		var err error
		sess, err = h.sessions.Create(w)
		if err != nil {
			http.Error(w, "session error", http.StatusInternalServerError)
			return
		}
	}

	state, err := randomHex(16)
	if err != nil {
		http.Error(w, "state generation error", http.StatusInternalServerError)
		return
	}
	sess.OAuthState = state
	sess.RedirectAfterAuth = r.URL.Query().Get("redirect")
	h.sessions.Save(w, sess)

	authURL := h.oauthCfgObj.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// CallbackHandler handles the redirect from Onshape after authorization.
func (h *Handler) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	Logger.Log("callback handler", "state", r.URL.Query().Get("state"), "oauthState")

	sess, ok := h.sessions.Get(r)
	if !ok {
		http.Error(w, "no session found, please start the login flow again", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if state != sess.OAuthState {
		http.Error(w, "invalid OAuth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		// User denied access.
		http.Error(w, "authorization denied", http.StatusForbidden)
		return
	}

	token, err := h.oauthCfgObj.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, fmt.Sprintf("token exchange failed: %v", err), http.StatusInternalServerError)
		return
	}

	sess.AccessToken = token.AccessToken
	sess.RefreshToken = token.RefreshToken
	sess.OAuthState = ""
	redirect := sess.RedirectAfterAuth
	sess.RedirectAfterAuth = ""
	h.sessions.Save(w, sess)

	if redirect == "" {
		redirect = "/panel"
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

// LogoutHandler clears the session and cookie.
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.sessions.Delete(w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

// tokenResponse is the structure returned by the token refresh endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshToken exchanges a refresh token for a new access + refresh pair.
// It updates the session in place and returns the new access token.
func (h *Handler) RefreshToken(ctx context.Context, sess *Session, w http.ResponseWriter) (string, error) {
	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", sess.RefreshToken)
	body.Set("client_id", h.cfg.ClientID)
	body.Set("client_secret", h.cfg.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, onshapeTokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("refresh token request returned %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode refresh response: %w", err)
	}

	sess.AccessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		sess.RefreshToken = tr.RefreshToken
	}
	if w != nil {
		h.sessions.Save(w, sess)
	}
	return tr.AccessToken, nil
}

// Authenticated reports whether the session has a valid access token.
func Authenticated(sess *Session) bool {
	return sess != nil && sess.AccessToken != ""
}
