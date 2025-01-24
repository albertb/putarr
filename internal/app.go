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

	putioClient := newPutDotIoClient(ctx, options)

	myPutioClient := myputio.NewClient(options, putioClient)
	arrClient := arr.New(options,
		func() *radarr.Radarr {
			if options.Config.Radarr == nil {
				return nil
			}
			arrConfig := starr.New(options.Config.Radarr.APIKey, options.Config.Radarr.URL, 0)
			if options.Verbose {
				arrConfig.Client.Transport = &timingRoundTripper{arrConfig.Client.Transport}
			}
			return radarr.New(arrConfig)
		}(),
		func() *sonarr.Sonarr {
			if options.Config.Radarr == nil {
				return nil
			}
			arrConfig := starr.New(options.Config.Sonarr.APIKey, options.Config.Sonarr.URL, 0)
			if options.Verbose {
				arrConfig.Client.Transport = &timingRoundTripper{arrConfig.Client.Transport}
			}
			return sonarr.New(arrConfig)
		}())

	mux := handler.New(options, myPutioClient, arrClient)

	janitor := myputio.NewJanitor(options, myPutioClient, arrClient)
	janitor.Run()

	log.Println("Listening on", addr)
	return http.ListenAndServe(addr, mux)
}

func newPutDotIoClient(ctx context.Context, options *config.Options) *putio.Client {
	// TODO oob flow, see: https://github.com/davidchalifoux/kaput-cli/blob/main/src/put/oob.rs
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: options.Config.Putio.OAuthToken})
	oauthClient := oauth2.NewClient(ctx, tokenSource)

	if options.Verbose {
		oauthClient.Transport = &timingRoundTripper{oauthClient.Transport}
	}

	return putio.NewClient(oauthClient)
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
