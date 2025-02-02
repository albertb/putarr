package internal

import (
	"context"
	"fmt"

	"golift.io/starr"
	"golift.io/starr/radarr"
	"golift.io/starr/sonarr"
)

type ArrClient struct {
	config       *Config
	radarrClient *radarr.Radarr
	sonarrClient *sonarr.Sonarr
}

func NewArrClient(config *Config, radarrClient *radarr.Radarr, sonarrClient *sonarr.Sonarr) *ArrClient {
	return &ArrClient{
		config:       config,
		radarrClient: radarrClient,
		sonarrClient: sonarrClient,
	}
}

type RadarrStatus struct {
	StatusByMovieID map[int64]*RadarrItemStatus
}

type RadarrItemStatus struct {
	PendingRecord *radarr.QueueRecord
	ImportRecord  *radarr.HistoryRecord
}

type SonarrStatus struct {
	StatusByEpisodeID map[int64]*SonarrItemStatus
}

type SonarrItemStatus struct {
	PendingRecord *sonarr.QueueRecord
	ImportRecord  *sonarr.HistoryRecord
}

func (c *ArrClient) GetRadarrImportStatusByTransferID(ctx context.Context) (map[int64]*RadarrStatus, error) {
	result := map[int64]*RadarrStatus{}
	if c.radarrClient == nil {
		return result, nil
	}

	// Get the most recent queue records, this will include in-progress imports.
	queue, err := c.radarrClient.GetQueuePageContext(ctx, &starr.PageReq{
		PageSize: 1000,
		SortKey:  "date",
		SortDir:  "descending",
	})
	if err != nil {
		return result, fmt.Errorf("failed to get queue from Radarr: %w", err)
	}

	// Get the most recent history records for imported items.
	history, err := c.radarrClient.GetHistoryPageContext(ctx, &starr.PageReq{
		PageSize: 1000,
		SortKey:  "date",
		SortDir:  "descending",
		Filter:   radarr.FilterDownloadFolderImported,
	})
	if err != nil {
		return result, fmt.Errorf("failed to get history from Radarr: %w", err)
	}

	for _, record := range queue.Records {
		id, err := ParseTorrentHash(record.DownloadID)
		if err != nil {
			continue
		}

		status, ok := result[id]
		if !ok {
			status = &RadarrStatus{StatusByMovieID: map[int64]*RadarrItemStatus{}}
			result[id] = status
		}

		item, ok := status.StatusByMovieID[record.MovieID]
		if !ok {
			item = &RadarrItemStatus{}
			status.StatusByMovieID[record.MovieID] = item
		}
		item.PendingRecord = record
	}

	for _, record := range history.Records {
		id, err := ParseTorrentHash(record.DownloadID)
		if err != nil {
			continue
		}

		status, ok := result[id]
		if !ok {
			status = &RadarrStatus{StatusByMovieID: map[int64]*RadarrItemStatus{}}
			result[id] = status
		}

		item, ok := status.StatusByMovieID[record.MovieID]
		if !ok {
			item = &RadarrItemStatus{}
			status.StatusByMovieID[record.MovieID] = item
		}
		item.ImportRecord = record
	}

	return result, nil
}

func (c *ArrClient) GetSonarrImportStatusByTransferID(ctx context.Context) (map[int64]*SonarrStatus, error) {
	result := map[int64]*SonarrStatus{}
	if c.radarrClient == nil {
		return result, nil
	}

	// Get the most recent queue records, this will include in-progress imports.
	queue, err := c.sonarrClient.GetQueuePageContext(ctx, &starr.PageReq{
		PageSize: 1000,
		SortKey:  "date",
		SortDir:  "descending",
	})
	if err != nil {
		return result, fmt.Errorf("failed to get queue from Sonarr: %w", err)
	}

	// Get the most recent history records for imported items.
	history, err := c.sonarrClient.GetHistoryPageContext(ctx, &starr.PageReq{
		PageSize: 1000,
		SortKey:  "date",
		SortDir:  "descending",
		Filter:   sonarr.FilterDownloadFolderImported,
	})
	if err != nil {
		return result, fmt.Errorf("failed to get history from Sonarr: %w", err)
	}

	for _, record := range queue.Records {
		id, err := ParseTorrentHash(record.DownloadID)
		if err != nil {
			continue
		}

		status, ok := result[id]
		if !ok {
			status = &SonarrStatus{StatusByEpisodeID: map[int64]*SonarrItemStatus{}}
			result[id] = status
		}

		item, ok := status.StatusByEpisodeID[record.EpisodeID]
		if !ok {
			item = &SonarrItemStatus{}
			status.StatusByEpisodeID[record.EpisodeID] = item
		}
		item.PendingRecord = record
	}

	for _, record := range history.Records {
		id, err := ParseTorrentHash(record.DownloadID)
		if err != nil {
			continue
		}

		status, ok := result[id]
		if !ok {
			status = &SonarrStatus{StatusByEpisodeID: map[int64]*SonarrItemStatus{}}
			result[id] = status
		}

		item, ok := status.StatusByEpisodeID[record.EpisodeID]
		if !ok {
			item = &SonarrItemStatus{}
			status.StatusByEpisodeID[record.EpisodeID] = item
		}
		item.ImportRecord = record
	}

	return result, nil
}
