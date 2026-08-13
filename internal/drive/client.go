package drive

import (
	"sync"
	"time"

	"magnet-agg/internal/cfg"
	"magnet-agg/internal/classify"
	"magnet-agg/internal/tmdb"

	"github.com/halalcloud/golang-sdk-lite/halalcloud/apiclient"
	"github.com/halalcloud/golang-sdk-lite/halalcloud/config"
	"github.com/halalcloud/golang-sdk-lite/halalcloud/services/oauth"
	"github.com/halalcloud/golang-sdk-lite/halalcloud/services/offline"
	"github.com/halalcloud/golang-sdk-lite/halalcloud/services/userfile"
)

const apiHost = "openapi.2dland.cn"

type Client struct {
	mu       sync.RWMutex
	api      *apiclient.Client
	store    config.ConfigStore
	oauth    *oauth.OAuthService
	offline  *offline.OfflineTaskService
	userfile *userfile.UserFileService
	tmdb     *tmdb.Client
	ai       *classify.Client
	baseDir  string
	clientID string

	deviceCode  string
	loginExpire time.Time
}

type snapshot struct {
	api      *apiclient.Client
	oauth    *oauth.OAuthService
	offline  *offline.OfflineTaskService
	userfile *userfile.UserFileService
	tmdb     *tmdb.Client
	ai       *classify.Client
	store    config.ConfigStore
	baseDir  string
	clientID string
}

func (c *Client) snap() snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return snapshot{
		api: c.api, oauth: c.oauth, offline: c.offline, userfile: c.userfile,
		tmdb: c.tmdb, ai: c.ai, store: c.store, baseDir: c.baseDir, clientID: c.clientID,
	}
}

func New(c *cfg.Config) *Client {
	store := config.NewLocalFileConfigStore(c.TokenFile)
	api := apiclient.NewClient(nil, apiHost, c.ClientID, c.ClientSecret, store,
		apiclient.WithTimeout(30*time.Second),
	)
	cl := &Client{
		api:      api,
		store:    store,
		oauth:    oauth.NewOAuthService(api),
		offline:  offline.NewOfflineTaskService(api),
		userfile: userfile.NewUserFileService(api),
		baseDir:  c.BaseDir,
		clientID: c.ClientID,
	}
	if c.TmdbAPIKey != "" {
		cl.tmdb = tmdb.New(c.TmdbAPIKey, c.TmdbProxy, c.TmdbLang)
	}
	if c.AIAPIKey != "" && c.AIBaseURL != "" {
		cl.ai = classify.New(c.AIBaseURL, c.AIAPIKey, c.AIModel)
	}
	return cl
}

func (c *Client) Reload(cfg *cfg.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseDir = cfg.BaseDir
	if cfg.TmdbAPIKey == "" {
		c.tmdb = nil
	} else {
		c.tmdb = tmdb.New(cfg.TmdbAPIKey, cfg.TmdbProxy, cfg.TmdbLang)
	}
	if cfg.AIAPIKey == "" || cfg.AIBaseURL == "" {
		c.ai = nil
	} else {
		c.ai = classify.New(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
	}
	if cfg.ClientID != c.clientID {
		_ = c.store.ClearConfigs()
		c.clientID = cfg.ClientID
		c.api = apiclient.NewClient(nil, apiHost, cfg.ClientID, cfg.ClientSecret, c.store,
			apiclient.WithTimeout(30*time.Second),
		)
		c.oauth = oauth.NewOAuthService(c.api)
		c.offline = offline.NewOfflineTaskService(c.api)
		c.userfile = userfile.NewUserFileService(c.api)
		c.deviceCode = ""
	}
}

func (c *Client) UpdateCredentials(clientID, clientSecret string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.store.ClearConfigs()
	c.clientID = clientID
	c.api = apiclient.NewClient(nil, apiHost, clientID, clientSecret, c.store,
		apiclient.WithTimeout(30*time.Second),
	)
	c.oauth = oauth.NewOAuthService(c.api)
	c.offline = offline.NewOfflineTaskService(c.api)
	c.userfile = userfile.NewUserFileService(c.api)
	c.deviceCode = ""
}

func (c *Client) Logout() {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.store.ClearConfigs()
	c.api.AccessToken = ""
	c.deviceCode = ""
}

func (c *Client) HasCredentials() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientID != ""
}
