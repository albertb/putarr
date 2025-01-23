package handler

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/albertb/putarr/internal/arr"
	"github.com/albertb/putarr/internal/config"
	"github.com/albertb/putarr/internal/putio"
	"github.com/albertb/putarr/web"
)

type statusHandler struct {
	options     *config.Options
	putioClient *putio.Client
	arrClient   *arr.Client
}

func newStatusHandler(options *config.Options, putioClient *putio.Client, arrClient *arr.Client) *statusHandler {
	return &statusHandler{
		options:     options,
		putioClient: putioClient,
		arrClient:   arrClient,
	}
}

func (h *statusHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /", h.home)
	mux.HandleFunc("GET /transfers", h.transfers)
}

func (h *statusHandler) home(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(web.Templates, "templates/home.go.html")
	//tmpl, err := template.ParseFiles("web/templates/home.go.html")
	if err != nil {
		log.Println("failed to load templates", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "home", nil)
	if err != nil {
		log.Println("failed to execute templates", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *statusHandler) transfers(w http.ResponseWriter, r *http.Request) {
	transfers, err := h.assembleTransferList(r.Context())
	if err != nil {
		log.Println("failed to get transfer status", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFS(web.Templates, "templates/transfers.go.html")
	//tmpl, err := template.ParseFiles("web/templates/transfers.go.html")
	if err != nil {
		log.Println("failed to load templates", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = tmpl.ExecuteTemplate(w, "transfers", transfers)
	if err != nil {
		log.Println("failed to execute templates", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type transferStatus struct {
	Title       string
	Status      string
	PercentDone int
	Imports     []importItem
	Buttons     []button
}

type importItem struct {
	Label string
}

type button struct {
	Label string
	URL   string
}

type byLabel []importItem

func (a byLabel) Len() int           { return len(a) }
func (a byLabel) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byLabel) Less(i, j int) bool { return a[i].Label < a[j].Label }

func (h *statusHandler) assembleTransferList(ctx context.Context) ([]transferStatus, error) {
	transfers, err := h.putioClient.GetTransfers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfers from Put.io: %w", err)
	}

	radarrStatus, err := h.arrClient.GetRadarrImportStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get imports from radarr: %w", err)
	}

	err = h.arrClient.GetRadarrImportMovie(ctx, radarrStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to get movie details from radarr: %w", err)
	}

	sonarrStatus, err := h.arrClient.GetSonarrImportStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get imports from sonarr: %w", err)
	}

	err = h.arrClient.GetSonarrImportEpisodes(ctx, sonarrStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to get episode details from sonarr: %w", err)
	}

	var result []transferStatus
	for _, transfer := range transfers {
		status := transferStatus{
			Title:       transfer.Name,
			PercentDone: transfer.PercentDone,
			Status:      strings.ToTitle(transfer.Status),
			Imports:     []importItem{},
			Buttons:     []button{},
		}

		if transfer.FinishedAt == nil {
			status.Status = fmt.Sprintf("%s, started %s", toTitleCase(transfer.Status), toTimeAgo(transfer.CreatedAt.Time))
			status.Buttons = append(status.Buttons, button{
				Label: "Put.io transfers",
				URL:   "https://app.put.io/transfers",
			})
		} else {
			status.Status = fmt.Sprintf("%s %s", toTitleCase(transfer.Status), toTimeAgo(transfer.FinishedAt.Time))
			status.Buttons = append(status.Buttons, button{
				Label: "Put.io files",
				URL:   fmt.Sprintf("https://app.put.io/files/%d", transfer.FileID),
			})
		}

		arrButtons := map[string]string{}

		if radarrStatus[transfer.ID] != nil {
			for _, item := range radarrStatus[transfer.ID].StatusByMovieID {
				status.Imports = append(status.Imports, importItem{
					Label: item.Movie.Title,
				})
				url, err := url.JoinPath(h.options.Config.Radarr.URL, "movie", strconv.FormatInt(item.Movie.TmdbID, 10))
				if err != nil {
					log.Println("failed to join URL path", err)
					continue
				}
				arrButtons[url] = "Radarr"
			}
		}

		if sonarrStatus[transfer.ID] != nil {
			for _, item := range sonarrStatus[transfer.ID].StatusByEpisodeID {
				status.Imports = append(status.Imports, importItem{
					Label: fmt.Sprintf("%0dx%02d %s", item.Episode.SeasonNumber, item.Episode.EpisodeNumber, item.Episode.Title),
				})
				url, err := url.JoinPath(h.options.Config.Sonarr.URL, "series", item.Episode.Series.TitleSlug)
				if err != nil {
					log.Println("failed to join URL path", err)
					continue
				}
				arrButtons[url] = "Sonarr"
			}
		}

		sort.Sort(byLabel(status.Imports))

		for url, label := range arrButtons {
			status.Buttons = append(status.Buttons, button{
				Label: label,
				URL:   url,
			})
		}

		result = append(result, status)
	}
	return result, nil
}

func toTitleCase(str string) string {
	words := strings.Fields(str)
	for i, word := range words {
		words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
	}
	return strings.Join(words, " ")
}

func toTimeAgo(t time.Time) string {
	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(diff.Hours()))
	case diff < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", int(diff.Hours()/24))
	}
}
