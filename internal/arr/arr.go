package arr

import (
	"context"
	"fmt"

	"github.com/albertb/putarr/internal/config"
	"github.com/albertb/putarr/internal/transmission"
	"golift.io/starr"
	"golift.io/starr/radarr"
	"golift.io/starr/sonarr"
)

type Client struct {
	options      *config.Options
	radarrClient *radarr.Radarr
	sonarrClient *sonarr.Sonarr
}

func New(options *config.Options, radarrClient *radarr.Radarr, sonarrClient *sonarr.Sonarr) *Client {
	return &Client{
		options:      options,
		radarrClient: radarrClient,
		sonarrClient: sonarrClient,
	}
}

// RadarrStatus represents the status of all items associated with a single transfer.
type RadarrStatus struct {
	StatusByMovieID map[int64]*RadarrItemStatus
}

// In-progress (queue) and completed (history) imports related to single transfer.
type RadarrItemStatus struct {
	*radarr.QueueRecord
	*radarr.HistoryRecord
	*radarr.Movie
}

// IsDone checks whether every item of associated with a transfer has been imported.
func (s RadarrStatus) IsDone() bool {
	for _, status := range s.StatusByMovieID {
		if status.QueueRecord != nil {
			return false
		}
	}
	return true
}

func (s RadarrItemStatus) GetMovieID() int64 {
	if s.QueueRecord != nil {
		return s.QueueRecord.MovieID
	} else if s.HistoryRecord != nil {
		return s.HistoryRecord.MovieID
	}
	return 0
}

// In-progress (queue) and completed (history) imports related to single transfer.
type SonarrStatus struct {
	StatusByEpisodeID map[int64]*SonarrItemStatus
}

type SonarrItemStatus struct {
	*sonarr.QueueRecord
	*sonarr.HistoryRecord
	*sonarr.Episode
}

func (s SonarrStatus) IsDone() bool {
	for _, status := range s.StatusByEpisodeID {
		if status.QueueRecord != nil {
			return false
		}
	}
	return true
}

func (s SonarrItemStatus) GetEpisodeID() int64 {
	if s.QueueRecord != nil {
		return s.QueueRecord.EpisodeID
	} else if s.HistoryRecord != nil {
		return s.HistoryRecord.EpisodeID
	}
	return 0
}

func (c *Client) GetRadarrImportStatus(ctx context.Context) (map[int64]*RadarrStatus, error) {
	if c.radarrClient == nil {
		return map[int64]*RadarrStatus{}, nil
	}

	// Get the 1000 most recent records from the queue of in-progress imports.
	queue, err := c.radarrClient.GetQueuePageContext(ctx, &starr.PageReq{
		PageSize: 1000,
		SortKey:  "date",
		SortDir:  "descending",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get queue from Radarr: %w", err)
	}

	// Get the 1000 most recent history records for imported items.
	history, err := c.radarrClient.GetHistoryPageContext(ctx, &starr.PageReq{
		PageSize: 1000,
		SortKey:  "date",
		SortDir:  "descending",
		Filter:   radarr.FilterDownloadFolderImported,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get history from Radarr: %w", err)
	}

	result := map[int64]*RadarrStatus{}
	for _, record := range queue.Records {
		id, err := transmission.ParseTorrentHash(record.DownloadID)
		if err != nil {
			continue // We're not responsible for this import.
		}

		status, ok := result[id]
		if !ok {
			status = &RadarrStatus{
				StatusByMovieID: map[int64]*RadarrItemStatus{}}
			result[id] = status
		}

		item, ok := status.StatusByMovieID[record.MovieID]
		if !ok {
			item = &RadarrItemStatus{}
			status.StatusByMovieID[record.MovieID] = item
		}
		item.QueueRecord = record
	}

	for _, record := range history.Records {
		id, err := transmission.ParseTorrentHash(record.DownloadID)
		if err != nil {
			continue // We're not responsible for this import.
		}

		status, ok := result[id]
		if !ok {
			status = &RadarrStatus{
				StatusByMovieID: map[int64]*RadarrItemStatus{}}
			result[id] = status
		}

		item, ok := status.StatusByMovieID[record.MovieID]
		if !ok {
			item = &RadarrItemStatus{}
			status.StatusByMovieID[record.MovieID] = item
		}
		item.HistoryRecord = record
	}

	return result, nil
}

func (c *Client) GetRadarrImportMovie(ctx context.Context, statuses map[int64]*RadarrStatus) error {
	if c.radarrClient == nil {
		return nil
	}

	for _, status := range statuses {
		for _, item := range status.StatusByMovieID {
			movie, err := c.radarrClient.GetMovieByIDContext(ctx, item.GetMovieID())
			if err != nil {
				return fmt.Errorf("failed to get movie details: %w", err)
			}
			item.Movie = movie
		}
	}
	return nil
}

func (c *Client) GetSonarrImportStatus(ctx context.Context) (map[int64]*SonarrStatus, error) {
	if c.sonarrClient == nil {
		return map[int64]*SonarrStatus{}, nil
	}

	// Fetch the queue of episodes that are in the process of being imported.
	queue, err := c.sonarrClient.GetQueuePageContext(ctx, &starr.PageReq{
		PageSize: 1000,
		SortKey:  "date",
		SortDir:  "descending",
	})
	if err != nil {
		return map[int64]*SonarrStatus{}, fmt.Errorf("failed to get queue from Sonarr: %w", err)
	}

	// Fetch the history of episodes that have been imported succesfully.
	history, err := c.sonarrClient.GetHistoryPageContext(ctx, &starr.PageReq{
		PageSize: 1000,
		SortKey:  "date",
		SortDir:  "descending",
		Filter:   sonarr.FilterDownloadFolderImported,
	})
	if err != nil {
		return map[int64]*SonarrStatus{}, fmt.Errorf("failed to get history from Sonarr: %w", err)
	}

	result := map[int64]*SonarrStatus{}
	for _, record := range queue.Records {
		id, err := transmission.ParseTorrentHash(record.DownloadID)
		if err != nil {
			continue // We're not responsible for this import.
		}

		status, ok := result[id]
		if !ok {
			status = &SonarrStatus{
				StatusByEpisodeID: map[int64]*SonarrItemStatus{}}
			result[id] = status
		}

		item, ok := status.StatusByEpisodeID[record.EpisodeID]
		if !ok {
			item = &SonarrItemStatus{}
			status.StatusByEpisodeID[record.EpisodeID] = item
		}
		item.QueueRecord = record
	}

	for _, record := range history.Records {
		id, err := transmission.ParseTorrentHash(record.DownloadID)
		if err != nil {
			continue // We're not responsible for this import.
		}

		status, ok := result[id]
		if !ok {
			status = &SonarrStatus{
				StatusByEpisodeID: map[int64]*SonarrItemStatus{}}
			result[id] = status
		}

		item, ok := status.StatusByEpisodeID[record.EpisodeID]
		if !ok {
			item = &SonarrItemStatus{}
			status.StatusByEpisodeID[record.EpisodeID] = item
		}
		item.HistoryRecord = record
	}

	return result, nil
}

func (c *Client) GetSonarrImportEpisodes(ctx context.Context, statuses map[int64]*SonarrStatus) error {
	if c.sonarrClient == nil {
		return nil
	}

	for _, status := range statuses {
		for _, item := range status.StatusByEpisodeID {
			episode, err := c.sonarrClient.GetEpisodeByIDContext(ctx, item.GetEpisodeID())
			if err != nil {
				return fmt.Errorf("failed to get episode details: %w", err)
			}
			item.Episode = episode
		}
	}

	return nil
}
