package internal

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

func NewServer(config *Config, token string, putioProxy *PutioProxy) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /transmission/rpc", http.HandlerFunc(
		// No-op. This is called by the client to get the session ID token which is handled in the middleware.
		func(w http.ResponseWriter, r *http.Request) {},
	))

	mux.Handle("POST /transmission/rpc", handlePostRPC(config.Transmission.DownloadDir, putioProxy))

	return basicAuthMiddleware(
		config.Transmission.Username,
		config.Transmission.Password,
		dumbSessionMiddleware(token, mux))
}

func handlePostRPC(downloadDir string, putioProxy *PutioProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			log.Println("failed to decode request:", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		var err error
		var result any
		switch request.Method {
		case "session-get":
			log.Println("session-get")
			result = Session{
				RPCVersion:  "18",
				Version:     "14.0.0",
				DownloadDir: downloadDir,
			}
		case "torrent-add":
			log.Println("torrent-add")
			// The download-dir argument is an optional string.
			dir, ok := request.Arguments["download-dir"].(string)
			if !ok {
				// Use the default dir when one isn't specified in the request.
				dir = downloadDir
			}

			var transfer Transfer
			if filename, ok := request.Arguments["filename"].(string); ok {
				// The filename argument is a string that contains a magnet URL.
				transfer, err = putioProxy.AddTransfer(r.Context(), filename, dir)
				if err != nil {
					log.Println("failed to add transfer to Put.io:", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
			} else if metainfo, ok := request.Arguments["metainfo"].(string); ok {
				// The metainfo argument is a string that contains a Base64-encoded torrent file.
				torrent, err := base64.StdEncoding.DecodeString(metainfo)
				if err != nil {
					log.Println("failed to decode metainfo:", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
				transfer, err = putioProxy.UploadTorrent(r.Context(), torrent, dir)
				if err != nil {
					log.Println("failed to upload torrent to Put.io:", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
			} else {
				log.Printf("expected either filename or metainfo arguments. Request: %+v", request)
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			result = convertFromPutioTransfer(transfer)
		case "torrent-get":
			log.Println("torrent-get")
			transfers, err := putioProxy.GetTransfers(r.Context())
			if err != nil {
				log.Println("failed to list Put.io transfers:", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			torrents := []Torrent{}
			for _, transfer := range transfers {
				torrents = append(torrents, convertFromPutioTransfer(transfer))
			}
			result = map[string][]Torrent{"torrents": torrents}
		case "torrent-remove":
			log.Println("torrent-remove")
			deleteFiles, transferIDs, err := parseTorrentRemoveArgs(request.Arguments)
			if err != nil {
				log.Println("failed to parse arguments:", err)
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			err = putioProxy.RemoveTransfers(r.Context(), deleteFiles, transferIDs...)
			if err != nil {
				log.Println("failed to remove transfers on Put.io:", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		default:
			log.Printf("unexpected method: %+v", request)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		arguments, err := json.Marshal(result)
		if err != nil {
			log.Println("failed to encode response arguments:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		response := Response{
			Result:    "success",
			Arguments: arguments,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Println("failed to encode response:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	})
}

func parseTorrentRemoveArgs(args map[string]any) (bool, []int64, error) {
	var transferIDs []int64

	deleteFiles, ok := args["delete-local-data"].(bool)
	if !ok {
		return deleteFiles, transferIDs, errors.New("missing `delete-local-data` argument")
	}

	ids, ok := args["ids"].([]any)
	if !ok {
		return deleteFiles, transferIDs, errors.New("missing `ids` argument")
	}

	for _, id := range ids {
		hash, ok := id.(string)
		if !ok {
			return deleteFiles, transferIDs, fmt.Errorf("unrecognied ID type: %v", hash)
		}
		transferID, err := ParseTorrentHash(hash)
		if err != nil {
			return deleteFiles, transferIDs, fmt.Errorf("failed to parse torrent hash: %w", err)
		}
		transferIDs = append(transferIDs, transferID)
	}

	return deleteFiles, transferIDs, nil
}

// BasicAuthMiddleware fails requests that are missing the correct Basic Auth credentials.
func basicAuthMiddleware(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// DumbSessionMiddleware fails requests that are missing the specified token. The error specifies the expected token.
func dumbSessionMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != token {
			w.Header().Add("X-Transmission-Session-Id", token)
			http.Error(w, "Conflict", http.StatusConflict)
			return
		}
		next.ServeHTTP(w, r)
	})
}
