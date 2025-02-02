package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/albertb/putarr/internal"
	"github.com/putdotio/go-putio"
	"golang.org/x/oauth2"
	"golift.io/starr"
	"golift.io/starr/radarr"
	"golift.io/starr/sonarr"
)

func run(addr string, config *internal.Config) error {
	ctx := context.Background()

	putioClient := newPutioClient(ctx, config)
	arrClient := newArrClient(config)

	var downloader internal.Downloader
	if config.Downloader.Dir != "" {
		downloader = internal.NewDownloader(config, putioClient)
	}

	putioProxy := internal.NewPutioProxy(config, putioClient, downloader)

	janitor := internal.NewPutioJanitor(arrClient, putioProxy, downloader)
	janitor.RunAtInterval(ctx, config.Putio.JanitorInterval)

	log.Println("listening on", addr)

	s := internal.NewServer(config, "whatever", putioProxy, downloader)
	return http.ListenAndServe(addr, s)
}

func newPutioClient(ctx context.Context, config *internal.Config) *putio.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.Putio.OAuthToken})
	oauthClient := oauth2.NewClient(ctx, tokenSource)
	return putio.NewClient(oauthClient)
}

func newArrClient(config *internal.Config) *internal.ArrClient {
	var radarrClient *radarr.Radarr
	if config.Radarr != nil {
		radarrClient = radarr.New(starr.New(config.Radarr.APIKey, config.Radarr.URL, 0))
	}
	var sonarrClient *sonarr.Sonarr
	if config.Sonarr != nil {
		sonarrClient = sonarr.New(starr.New(config.Sonarr.APIKey, config.Sonarr.URL, 0))
	}
	return internal.NewArrClient(config, radarrClient, sonarrClient)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("failed to get user home directory: ", err)
	}
	defaultConfigPath := filepath.Join(home, ".config", "putarr", "config.yaml")

	addr := flag.String("addr", ":9091", "`address` to listen on")
	configPath := flag.String("config", defaultConfigPath, "configuration file")

	flag.Parse()

	file, err := os.Open(*configPath)
	if err != nil {
		log.Fatalln("failed to open config file:", err)
	}
	defer file.Close()

	config, err := internal.ReadConfig(file)
	if err != nil {
		log.Fatalln("failed to read config file:", err)

	}

	if err := run(*addr, &config); err != nil {
		log.Fatalln("failed to run server:", err)
	}
}
