package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/albertb/putarr/internal/config"
	"github.com/albertb/putarr/internal/putio"
	"github.com/albertb/putarr/internal/transmission"
)

type transmissionHandler struct {
	options     *config.Options
	putioClient *putio.Client
}

func newTransmissionHandler(options *config.Options, putioClient *putio.Client) *transmissionHandler {
	return &transmissionHandler{
		options:     options,
		putioClient: putioClient,
	}
}

func (h *transmissionHandler) Register(mux *http.ServeMux) {
	basicAuth := &basicAuthMiddleware{
		username: h.options.Config.Transmission.Username,
		password: h.options.Config.Transmission.Password,
	}
	mux.Handle("GET /transmission/rpc", basicAuth.Wrap(http.HandlerFunc(h.handleAuth)))
	mux.Handle("POST /transmission/rpc", basicAuth.Wrap(http.HandlerFunc(h.handleRPC)))
}

// Handles the Transmission API authentication.
func (h *transmissionHandler) handleAuth(w http.ResponseWriter, _ *http.Request) {
	// TODO Do we need a real Session ID here?
	w.Header().Add("X-Transmission-Session-Id", "whatever")
	http.Error(w, "whatever", http.StatusConflict)
}

// Handle the Transmission API RPCs.
func (h *transmissionHandler) handleRPC(w http.ResponseWriter, r *http.Request) {
	var req transmission.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println("failed to parse request:", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if h.options.Verbose {
		log.Println("req:", debugString(req))
	}

	var err error
	var result interface{}
	switch req.Method {
	case "session-get":
		result = h.getSession()
	case "torrent-get":
		result, err = h.getTransfers(r.Context())
	case "torrent-add":
		result, err = h.addTransfer(r.Context(), req.Arguments)
	case "torrent-remove":
		err = h.removeTransfers(r.Context(), req.Arguments)
	case "torrent-set", "queue-move-top":
		// TODO Should these be implemented too? No-ops for now.
		log.Println("IGNORING RPC", req.Method, debugString(req.Arguments))
	default:
		log.Println("unknown request method:", req.Method)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Println("failed to handle RPC:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resp := transmission.Response{
		Result:    "success",
		Arguments: result,
	}

	if h.options.Verbose {
		log.Println("res:", debugString(resp))
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Println("failed to encode response:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (h *transmissionHandler) getSession() transmission.Session {
	return transmission.Session{
		RPCVersion:  "18",
		Version:     "14.0.0",
		DownloadDir: h.options.Config.Transmission.DownloadDir,
	}
}

func (h *transmissionHandler) getTransfers(ctx context.Context) (map[string][]transmission.Torrent, error) {
	transfers, err := h.putioClient.GetTransfers(ctx)
	if err != nil {
		return nil, err
	}

	torrents := make([]transmission.Torrent, 0, len(transfers))
	for _, t := range transfers {
		torrents = append(torrents, convertFromPutioTransfer(t))
	}

	return map[string][]transmission.Torrent{
		"torrents": torrents,
	}, nil
}

func (h *transmissionHandler) addTransfer(ctx context.Context, args map[string]interface{}) (*transmission.Torrent, error) {
	var dir *string
	value, ok := args["download-dir"].(string)
	if ok {
		dir = &value
	}

	metainfo, ok := args["metainfo"].(string)
	if ok {
		// Metainfo holds the base64-encoded contents of a .torrent file.
		torrent, err := base64.StdEncoding.DecodeString(metainfo)
		if err != nil {
			return nil, fmt.Errorf("failed to decode metainfo: %w", err)
		}

		transfer, err := h.putioClient.UploadTorrent(ctx, torrent, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to upload torrent to Put.io: %w", err)
		}

		converted := convertFromPutioTransfer(*transfer)
		return &converted, nil
	}

	filename, ok := args["filename"].(string)
	if ok {
		// Filename holds a magnet URL.
		transfer, err := h.putioClient.AddTransfer(ctx, filename, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to add transfer to Put.io: %w", err)
		}

		converted := convertFromPutioTransfer(*transfer)
		return &converted, nil
	}

	return nil, errors.New("invalid torrent-add arguments; expecting either metainfo or filename")
}

func (h *transmissionHandler) removeTransfers(ctx context.Context, args map[string]interface{}) error {
	deleteFiles, ok := args["delete-local-data"].(bool)
	if !ok {
		return errors.New("invalid torrent-remove arguments; expecting delete-local-data")
	}

	idsArg, ok := args["ids"].([]interface{})
	if !ok {
		return errors.New("invalid torrent-remove arguments; expecting ids")
	}

	ids := make([]int64, 0, len(idsArg))
	for _, idArg := range idsArg {
		key, ok := idArg.(string)
		if !ok {
			return fmt.Errorf("unrecognized id type: %v", idArg)
		}
		id, err := transmission.ParseTorrentHash(key)
		if err != nil {
			return fmt.Errorf("failed to parse torrent hash: %w", err)
		}
		ids = append(ids, id)
	}
	return h.putioClient.RemoveTransfer(ctx, deleteFiles, ids)
}

func convertFromPutioTransfer(transfer putio.Transfer) transmission.Torrent {
	hash := transmission.FormatTorrentHash(transfer.ID)

	createdAt := time.Now()
	if transfer.CreatedAt != nil {
		createdAt = transfer.CreatedAt.Time
	}

	return transmission.Torrent{
		ID:                 int(transfer.ID),
		HashString:         &hash,
		Name:               transfer.Name,
		DownloadDir:        transfer.DownloadDir,
		TotalSize:          int64(transfer.Size),
		LeftUntilDone:      int64(transfer.Size) - transfer.Downloaded,
		IsFinished:         transfer.FinishedAt != nil,
		ETA:                transfer.EstimatedTime,
		Status:             transmission.ConvertFromPutioStatus(transfer.Status),
		SecondsDownloading: int64(time.Since(createdAt).Seconds()),
		ErrorString:        &transfer.ErrorMessage,
		DownloadedEver:     transfer.Downloaded,
		SeedRatioLimit:     0.0,
		SeedRatioMode:      0,
		SeedIdleLimit:      0,
		SeedIdleMode:       0,
		FileCount:          1,
	}
}

// Attempts to marshall whatever as json, and falls back to %+v when that isn't possible.
func debugString(whatever interface{}) string {
	json, err := json.MarshalIndent(whatever, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", whatever)
	}
	return string(json)
}

type basicAuthMiddleware struct {
	username string
	password string
}

func (b *basicAuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != b.username || password != b.password {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		// The credentials are valid, continue on to the next handler.
		next.ServeHTTP(w, r)
	})
}
