package config

import (
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"gopkg.in/yaml.v3"
)

type Options struct {
	Config      Config
	Verbose     bool // Whether to print verbose logs
	Development bool // Whether to run in development mode
}

type Config struct {
	Transmission TransmissionConfig `yaml:"transmission"` // Transmission-specific configuration.
	Putio        PutioConfig        `yaml:"putio"`        // Putio-specific configuration.
	Radarr       *RadarrConfig      `yaml:"radarr"`       // Radarr-specific configuration.
	Sonarr       *SonarrConfig      `yaml:"sonarr"`       // Sonarr-specific configuration.
}

type TransmissionConfig struct {
	Username    string `yaml:"username"`     // Username to communicate with this server.
	Password    string `yaml:"password"`     // Password to communicate with this server.
	DownloadDir string `yaml:"download_dir"` // The download directory to report to transmission clients.
}

type PutioConfig struct {
	OAuthToken      string        `yaml:"oauth_token"`      // OAuth token to communicate with Put.io.
	ParentDirID     int64         `yaml:"parent_dir_id"`    // Parent directory for new Put.io transfers, leave unset to use the default.
	JanitorInterval time.Duration `yaml:"janitor_interval"` // How often to run the janitor to remove imported files.

	// When multiple users are sharing a single Put.io account, the friend token is used to disambiguate transfer
	// ownership. When this is left unset, all Putarr initiated transfers on the Put.io account are assumed to belong to
	// a single user.
	FriendToken string `yaml:"friend_token"`
}

func (p PutioConfig) JanitorIntervalOrDefault() time.Duration {
	if p.JanitorInterval < 10*time.Minute || p.JanitorInterval > 24*time.Hour {
		// If the field is unset or out-of-bound, use a sane default value.
		log.Println("JanitorInterval is invalid:", p.JanitorInterval, "Overriding value to 1 hour")
		return 1 * time.Hour
	}
	return p.JanitorInterval
}

type RadarrConfig struct {
	URL    string `yaml:"url"` // URL of the Radarr server.
	APIKey string `yaml:"api_key"`
}

type SonarrConfig struct {
	URL    string `yaml:"url"` // URL of the Sonarr server.
	APIKey string `yaml:"api_key"`
}

func Read(reader io.Reader) (*Config, error) {
	var c Config
	err := yaml.NewDecoder(reader).Decode(&c)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return c.validate()
}

func (c Config) validate() (*Config, error) {
	if c.Transmission.Username == "" {
		return nil, errors.New("transmission username is required")
	}
	if c.Transmission.Password == "" {
		return nil, errors.New("transmission password is required")
	}
	if c.Transmission.DownloadDir == "" {
		return nil, errors.New("transmission download_dir is required")
	}

	if c.Putio.OAuthToken == "" {
		return nil, errors.New("putio oauth_token is required")
	}

	if c.Radarr == nil && c.Sonarr == nil {
		return nil, errors.New("either radarr or sonarr configuration is required")
	}

	if c.Radarr != nil {
		if c.Radarr.URL == "" {
			return nil, errors.New("radarr url is required")
		}
		if c.Radarr.APIKey == "" {
			return nil, errors.New("radarr api_key is required")
		}
	}

	if c.Sonarr != nil {
		if c.Sonarr.URL == "" {
			return nil, errors.New("sonarr url is required")
		}
		if c.Sonarr.APIKey == "" {
			return nil, errors.New("sonarr api_key is required")
		}
	}
	return &c, nil
}
