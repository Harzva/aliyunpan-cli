package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tickstep/aliyunpan-api/aliyunpan_open/openapi"
)

const (
	defaultAuthorizeURL  = "https://openapi.alipan.com/oauth/authorize"
	defaultTokenEndpoint = "https://openapi.alipan.com/oauth/access_token"
	defaultScope         = "user:base,file:all:read,file:all:write"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpireIn     int64  `json:"expire_in"`
	ExpiresTime  string `json:"expires_time"`
	TokenType    string `json:"token_type"`
	ExpiredAt    int64  `json:"expired_at"`
	Message      string `json:"message"`
	Code         string `json:"code"`
}

func (a *App) runAuth(args []string, opts OutputOptions) error {
	if len(args) == 0 {
		return usageError("auth requires a subcommand: login or import")
	}
	switch args[0] {
	case "login":
		return a.runAuthLogin(args[1:], opts)
	case "import":
		return a.runAuthImport(args[1:], opts)
	default:
		return usageError("unknown auth subcommand %q", args[0])
	}
}

func (a *App) runAuthLogin(args []string, opts OutputOptions) error {
	fs := newFlagSet("auth login", a.errOut, &opts)
	clientID := fs.String("client-id", os.Getenv("ALIYUNPAN_CLIENT_ID"), "Aliyun Drive OpenAPI client id")
	clientSecret := fs.String("client-secret", os.Getenv("ALIYUNPAN_CLIENT_SECRET"), "Aliyun Drive OpenAPI client secret")
	redirectURI := fs.String("redirect-uri", "oob", "OAuth redirect URI")
	scope := fs.String("scope", defaultScope, "OAuth scope")
	code := fs.String("code", "", "authorization code")
	codeVerifier := fs.String("code-verifier", "", "PKCE code verifier")
	authorizeURL := fs.String("authorize-url", defaultAuthorizeURL, "OAuth authorization URL")
	tokenEndpoint := fs.String("token-endpoint", defaultTokenEndpoint, "OAuth token endpoint")
	profileName := fs.String("profile", "default", "local profile name")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	if *clientID == "" {
		return usageError("auth login requires --client-id or ALIYUNPAN_CLIENT_ID")
	}
	if *code == "" {
		u, err := buildAuthorizeURL(*authorizeURL, *clientID, *redirectURI, *scope, *codeVerifier)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.errOut, "Open this URL and authorize the app:\n%s\n\nPaste authorization code: ", u)
		scanner := bufio.NewScanner(a.in)
		if !scanner.Scan() {
			return authError("no authorization code provided")
		}
		*code = strings.TrimSpace(scanner.Text())
	}

	tok, err := exchangeToken(*tokenEndpoint, map[string]any{
		"grant_type":    "authorization_code",
		"client_id":     *clientID,
		"client_secret": *clientSecret,
		"code":          *code,
		"redirect_uri":  *redirectURI,
		"code_verifier": *codeVerifier,
	})
	if err != nil {
		return err
	}
	expiresAt := tokenExpiry(tok)
	profile := &Profile{
		Name:          *profileName,
		Source:        "oauth",
		AccessToken:   tok.AccessToken,
		RefreshToken:  tok.RefreshToken,
		ExpiresAt:     expiresAt,
		ClientID:      *clientID,
		ClientSecret:  *clientSecret,
		TokenEndpoint: *tokenEndpoint,
		ImportedAt:    nowRFC3339(),
	}
	if err := enrichProfile(profile); err != nil {
		return err
	}
	cfg, path, err := loadConfig(opts.ConfigDir)
	if err != nil {
		return wrapError("config_error", "failed to load config", exitFilesystem, err)
	}
	cfg.Profiles[profile.Name] = profile
	cfg.ActiveProfile = profile.Name
	if err := saveConfig(path, cfg); err != nil {
		return wrapError("config_error", "failed to save config", exitFilesystem, err)
	}
	return writeOutput(a.out, opts.Format, profile.public())
}

func buildAuthorizeURL(base, clientID, redirectURI, scope, verifier string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", usageError("invalid authorize URL: %v", err)
	}
	q := u.Query()
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scope)
	q.Set("response_type", "code")
	if verifier != "" {
		q.Set("code_challenge", verifier)
		q.Set("code_challenge_method", "plain")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func exchangeToken(endpoint string, payload map[string]any) (*tokenResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, wrapError("auth_error", "failed to encode token request", exitAuth, err)
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, wrapError("auth_error", "failed to build token request", exitAuth, err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, wrapError("auth_error", "token request failed", exitAuth, err)
	}
	defer resp.Body.Close()
	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, wrapError("auth_error", "failed to parse token response", exitAuth, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := tok.Message
		if msg == "" {
			msg = resp.Status
		}
		return nil, authError("token endpoint rejected request: %s", msg)
	}
	if tok.AccessToken == "" {
		return nil, authError("token response did not include access_token")
	}
	return &tok, nil
}

func tokenExpiry(tok *tokenResponse) int64 {
	if tok.ExpiredAt > 0 {
		return tok.ExpiredAt
	}
	if tok.ExpiresIn > 0 {
		return time.Now().Unix() + tok.ExpiresIn
	}
	if tok.ExpireIn > 0 {
		return time.Now().Unix() + tok.ExpireIn
	}
	if tok.ExpiresTime != "" {
		if t, err := time.Parse(time.RFC3339Nano, tok.ExpiresTime); err == nil {
			return t.Unix()
		}
	}
	return time.Now().Add(2 * time.Hour).Unix()
}

func refreshProfileToken(profile *Profile) error {
	if profile.RefreshToken == "" {
		return authError("profile %q has no refresh token; run auth login again", profile.Name)
	}
	if profile.TokenEndpoint == "" {
		profile.TokenEndpoint = defaultTokenEndpoint
	}
	tok, err := exchangeToken(profile.TokenEndpoint, map[string]any{
		"grant_type":    "refresh_token",
		"client_id":     profile.ClientID,
		"client_secret": profile.ClientSecret,
		"refresh_token": profile.RefreshToken,
	})
	if err != nil {
		return err
	}
	profile.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		profile.RefreshToken = tok.RefreshToken
	}
	profile.ExpiresAt = tokenExpiry(tok)
	return nil
}

func maybeRefreshProfile(profile *Profile, cfg *Config, cfgPath string) error {
	if profile.ExpiresAt == 0 || profile.ExpiresAt > time.Now().Add(5*time.Minute).Unix() {
		return nil
	}
	if profile.RefreshToken == "" {
		return authError("profile %q access token appears expired and no refresh token is available", profile.Name)
	}
	if err := refreshProfileToken(profile); err != nil {
		return err
	}
	return saveConfig(cfgPath, cfg)
}

type tickstepConfig struct {
	ActiveUID    string          `json:"activeUID"`
	UserList     []tickstepUser  `json:"userList"`
	ClientID     string          `json:"clientId"`
	ClientSecret string          `json:"clientSecret"`
	Extra        json.RawMessage `json:"-"`
}

type tickstepUser struct {
	UserID        string          `json:"userId"`
	Nickname      string          `json:"nickname"`
	AccountName   string          `json:"accountName"`
	ActiveDriveID string          `json:"activeDriveId"`
	TicketID      string          `json:"ticketId"`
	OpenAPIToken  *tickstepToken  `json:"openapiToken"`
	DriveList     []DriveInfo     `json:"driveList"`
	Extra         json.RawMessage `json:"-"`
}

type tickstepToken struct {
	AccessToken string `json:"accessToken"`
	Expired     int64  `json:"expired"`
}

func (a *App) runAuthImport(args []string, opts OutputOptions) error {
	fs := newFlagSet("auth import", a.errOut, &opts)
	from := fs.String("from", defaultTickstepConfigPath(), "tickstep aliyunpan_config.json path")
	userID := fs.String("user-id", "", "tickstep user id to import")
	profileName := fs.String("profile", "default", "local profile name")
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	data, err := os.ReadFile(*from)
	if err != nil {
		return wrapError("config_error", "failed to read tickstep config", exitFilesystem, err)
	}
	var src tickstepConfig
	if err := json.Unmarshal(data, &src); err != nil {
		return wrapError("config_error", "failed to parse tickstep config", exitFilesystem, err)
	}
	user := selectTickstepUser(src, *userID)
	if user == nil {
		return authError("no matching user found in tickstep config")
	}
	if user.OpenAPIToken == nil || user.OpenAPIToken.AccessToken == "" {
		return authError("selected tickstep user has no openapi token")
	}
	profile := &Profile{
		Name:          *profileName,
		Source:        "tickstep-import",
		UserID:        user.UserID,
		Nickname:      user.Nickname,
		AccountName:   user.AccountName,
		AccessToken:   user.OpenAPIToken.AccessToken,
		ExpiresAt:     user.OpenAPIToken.Expired,
		ClientID:      src.ClientID,
		ClientSecret:  src.ClientSecret,
		ActiveDriveID: user.ActiveDriveID,
		Drives:        user.DriveList,
		ImportedAt:    nowRFC3339(),
	}
	if profile.ActiveDriveID == "" && len(profile.Drives) > 0 {
		profile.ActiveDriveID = profile.Drives[0].DriveID
	}
	cfg, path, err := loadConfig(opts.ConfigDir)
	if err != nil {
		return wrapError("config_error", "failed to load config", exitFilesystem, err)
	}
	cfg.Profiles[profile.Name] = profile
	cfg.ActiveProfile = profile.Name
	if err := saveConfig(path, cfg); err != nil {
		return wrapError("config_error", "failed to save config", exitFilesystem, err)
	}
	return writeOutput(a.out, opts.Format, profile.public())
}

func defaultTickstepConfigPath() string {
	if dir := os.Getenv("ALIYUNPAN_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "aliyunpan_config.json")
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "aliyunpan", "aliyunpan_config.json")
	}
	return "aliyunpan_config.json"
}

func selectTickstepUser(src tickstepConfig, userID string) *tickstepUser {
	for i := range src.UserList {
		if userID != "" && src.UserList[i].UserID == userID {
			return &src.UserList[i]
		}
	}
	for i := range src.UserList {
		if userID == "" && src.ActiveUID != "" && src.UserList[i].UserID == src.ActiveUID {
			return &src.UserList[i]
		}
	}
	if userID == "" && len(src.UserList) > 0 {
		return &src.UserList[0]
	}
	return nil
}

func apiToken(profile *Profile) openapi.ApiToken {
	return openapi.ApiToken{AccessToken: profile.AccessToken, ExpiredAt: profile.ExpiresAt}
}
