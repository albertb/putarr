package internal

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/albertb/putarr/internal/arr"
	"github.com/albertb/putarr/internal/config"
	"github.com/albertb/putarr/internal/handler"
	myputio "github.com/albertb/putarr/internal/putio"
	"github.com/putdotio/go-putio"
	"golang.org/x/oauth2"
	"golift.io/starr"
	"golift.io/starr/radarr"
	"golift.io/starr/sonarr"
)

func Run(addr string, options *config.Options) error {
	ctx := context.Background()

	putioClient := newPutioClient(ctx, options)
	myPutioClient := myputio.NewClient(options, putioClient)
	arrClient := newArrClient(options)

	mux := handler.New(options, myPutioClient, arrClient)

	janitor := myputio.NewJanitor(options, myPutioClient, arrClient)
	janitor.Run()

	log.Println("Listening on", addr)
	return http.ListenAndServe(addr, mux)
}

func newPutioClient(ctx context.Context, options *config.Options) *putio.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: options.Config.Putio.OAuthToken})
	oauthClient := oauth2.NewClient(ctx, tokenSource)

	if options.Verbose {
		oauthClient.Transport = &timingRoundTripper{oauthClient.Transport}
	}

	return putio.NewClient(oauthClient)
}

func newArrClient(options *config.Options) *arr.Client {
	var radarrClient *radarr.Radarr
	if options.Config.Radarr != nil {
		arrConfig := starr.New(options.Config.Radarr.APIKey, options.Config.Radarr.URL, 0)
		if options.Verbose {
			arrConfig.Client.Transport = &timingRoundTripper{arrConfig.Client.Transport}
		}
		radarrClient = radarr.New(arrConfig)
	}

	var sonarrClient *sonarr.Sonarr
	if options.Config.Sonarr != nil {
		arrConfig := starr.New(options.Config.Sonarr.APIKey, options.Config.Sonarr.URL, 0)
		if options.Verbose {
			arrConfig.Client.Transport = &timingRoundTripper{arrConfig.Client.Transport}
		}
		sonarrClient = sonarr.New(arrConfig)
	}

	return arr.New(options, radarrClient, sonarrClient)
}

type timingRoundTripper struct {
	transport http.RoundTripper
}

func (t *timingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.transport.RoundTrip(req)
	log.Println("HTTP request took", time.Since(start), "URL:", req.URL)
	return resp, err
}
