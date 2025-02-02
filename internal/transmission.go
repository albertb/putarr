package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

type Request struct {
	Method    string                 `json:"method"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type Response struct {
	Result    string          `json:"result"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type Session struct {
	RPCVersion  string `json:"rpc-version"`
	Version     string `json:"version"`
	DownloadDir string `json:"download-dir"`
}

type Torrent struct {
	ID                 int           `json:"id"`
	HashString         *string       `json:"hashString"`
	Name               string        `json:"name"`
	DownloadDir        string        `json:"downloadDir"`
	TotalSize          int64         `json:"totalSize"`
	LeftUntilDone      int64         `json:"leftUntilDone"`
	IsFinished         bool          `json:"isFinished"`
	ETA                int64         `json:"eta"`
	Status             TorrentStatus `json:"status"`
	SecondsDownloading int64         `json:"secondsDownloading"`
	ErrorString        *string       `json:"errorString"`
	DownloadedEver     int64         `json:"downloadedEver"`
	SeedRatioLimit     float32       `json:"seedRatioLimit"`
	SeedRatioMode      int           `json:"seedRatioMode"`
	SeedIdleLimit      int64         `json:"seedIdleLimit"`
	SeedIdleMode       int           `json:"seedIdleMode"`
	FileCount          int           `json:"fileCount"`
}

type TorrentStatus int64

const (
	TorrentStatusStopped TorrentStatus = iota
	TorrentStatusCheckPending
	TorrentStatusChecking
	TorrentStatusDownloadPending
	TorrentStatusDownloading
	TorrentStatusSeedPending
	TorrentStatusSeeding
)

func convertFromPutioTransfer(transfer Transfer) Torrent {
	hash := FormatTorrentHash(transfer.ID)

	createdAt := time.Now()
	if transfer.CreatedAt != nil {
		createdAt = transfer.CreatedAt.Time
	}

	return Torrent{
		ID:                 int(transfer.ID),
		HashString:         &hash,
		Name:               transfer.Name,
		DownloadDir:        transfer.DownloadDir,
		TotalSize:          int64(transfer.Size),
		LeftUntilDone:      int64(transfer.Size) - transfer.Downloaded,
		IsFinished:         transfer.FinishedAt != nil,
		ETA:                transfer.EstimatedTime,
		Status:             ConvertFromPutioStatus(transfer.Status),
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

func ConvertFromPutioStatus(status string) TorrentStatus {
	switch strings.ToUpper(status) {
	case "COMPLETED", "ERROR":
		return TorrentStatusStopped
	case "PREPARING_DOWNLOAD":
		return TorrentStatusCheckPending
	case "COMPLETING":
		return TorrentStatusChecking
	case "IN_QUEUE":
		return TorrentStatusDownloadPending
	case "DOWNLOADING":
		return TorrentStatusDownloading
	case "WAITING":
		return TorrentStatusSeedPending
	case "SEEDING":
		return TorrentStatusSeeding
	default:
		log.Printf("unknown torrent status '%v'; defaulting to check_pending", status)
		return TorrentStatusCheckPending
	}
}

func FormatTorrentHash(id int64) string {
	return fmt.Sprintf("putarr;%d", id)
}

func ParseTorrentHash(key string) (int64, error) {
	// Clients (e.g., Sonarr/Radarr) sometimes make the hash uppercase.
	key = strings.ToLower(key)

	s, ok := strings.CutPrefix(key, "putarr;")
	if !ok {
		return 0, errors.New("invalid transfer key: " + key)
	}
	return strconv.ParseInt(s, 10, 64)
}
