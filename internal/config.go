package internal

import (
	"errors"
	"fmt"
	"io"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Downloader   DownloaderConfig   `yaml:"downloader"`
	Transmission TransmissionConfig `yaml:"transmission"`
	Putio        PutioConfig        `yaml:"putio"`
	Radarr       *RadarrConfig      `yaml:"radarr"`
	Sonarr       *SonarrConfig      `yaml:"sonarr"`
}

type DownloaderConfig struct {
	// Download directory from the point-of-view of Putarr. Leave this unset to disable local downloading.
	Dir string `yaml:"dir"`
}

type TransmissionConfig struct {
	Username    string `yaml:"username"`     // Username clients must use to communicate with this server.
	Password    string `yaml:"password"`     // Password clients must use to communicate with this server.
	DownloadDir string `yaml:"download_dir"` // Download directory to report to clients; this is the directory from the point-of-view of the *arrs.
}

type PutioConfig struct {
	OAuthToken      string        `yaml:"oauth_token"`      // Token to authenticate with Put.io.
	ParentDirID     int64         `yaml:"parent_dir_id"`    // Parent directory for new transfers on Put.io. Unset for default.
	JanitorInterval time.Duration `yaml:"janitor_interval"` // How often to run the janitor to remove completed transfers and files.

	// When multiple instances of Putiarr are using a single Put.io account, the friend token is used to disambiguate
	// transfer ownership. When this is left unset, all Putiarr initiated transfers on the Put.io account are assumed to
	// belong to a single instance.
	FriendToken string `yaml:"friend_token"`
}

type RadarrConfig struct {
	APIKey string `yaml:"api_key"`
	URL    string `yaml:"url"`
}

type SonarrConfig struct {
	APIKey string `yaml:"api_key"`
	URL    string `yaml:"url"`
}

func ReadConfig(reader io.Reader) (Config, error) {
	var config Config
	if err := yaml.NewDecoder(reader).Decode(&config); err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}
	return validate(config)
}

func validate(config Config) (Config, error) {
	if config.Transmission.Username == "" {
		return config, errors.New("transmission.username is required")
	}
	if config.Transmission.Password == "" {
		return config, errors.New("transmission.password is required")
	}
	if config.Transmission.DownloadDir == "" {
		return config, errors.New("transmission.download_dir is required")
	}

	if config.Putio.OAuthToken == "" {
		return config, errors.New("putio.oauth_token is required")
	}

	if config.Radarr == nil && config.Sonarr == nil {
		return config, errors.New("at least one of radarr or sonarr is required")
	}

	if c := config.Radarr; c != nil {
		if c.APIKey == "" {
			return config, errors.New("radarr.api_key is required")
		}
		if c.URL == "" {
			return config, errors.New("radarr.url is required")
		}
	}

	if c := config.Sonarr; c != nil {
		if c.APIKey == "" {
			return config, errors.New("sonarr.api_key is required")
		}
		if c.URL == "" {
			return config, errors.New("sonarr.url is required")
		}
	}

	return config, nil
}
