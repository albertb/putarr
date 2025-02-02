package internal

import (
	"context"
	"fmt"
	"log"
	"time"
)

type PutioJanitor struct {
	arrClient  *ArrClient
	putioProxy *PutioProxy
	downloader Downloader
}

func NewPutioJanitor(arrClient *ArrClient, putioProxy *PutioProxy, downloader Downloader) *PutioJanitor {
	return &PutioJanitor{
		arrClient:  arrClient,
		putioProxy: putioProxy,
		downloader: downloader,
	}
}

// RunAtInterval runs the janitor at the specified interval.
func (j *PutioJanitor) RunAtInterval(ctx context.Context, interval time.Duration) {
	ticker := time.Tick(interval)
	go func() {
		for {
			if _, err := j.RunOnce(ctx); err != nil {
				log.Println("failed to run janitor:", err)
			}
			<-ticker
		}
	}()
}

// RunOnce runs the janitor and returns the IDs of transfers that were cleaned up.
func (j *PutioJanitor) RunOnce(ctx context.Context) ([]int64, error) {
	completedTransferIDs := []int64{}

	transfers, err := j.putioProxy.GetTransfers(ctx)
	if err != nil {
		return completedTransferIDs, fmt.Errorf("failed to get transfers from Put.io: %w", err)
	}

	radarrStatuses, err := j.arrClient.GetRadarrImportStatusByTransferID(ctx)
	if err != nil {
		return completedTransferIDs, err
	}
	sonarrStatuses, err := j.arrClient.GetSonarrImportStatusByTransferID(ctx)
	if err != nil {
		return completedTransferIDs, err
	}

	// TODO DOESNT WORK

	// Find transfers with successful imports and no pending queue activities.
	for _, transfer := range transfers {
		imported := true

		// Items with queue records and/or no import records are not considered to be imported yet.
		if status, ok := radarrStatuses[transfer.ID]; ok {
			for _, item := range status.StatusByMovieID {
				if item.ImportRecord == nil || item.PendingRecord != nil {
					imported = false
				}
			}
		} else if status, ok := sonarrStatuses[transfer.ID]; ok {
			for _, item := range status.StatusByEpisodeID {
				if item.ImportRecord == nil || item.PendingRecord != nil {
					imported = false
				}
			}
		} else {
			log.Println("no corresponding imports for Put.io transfer with ID:", transfer.ID)
			continue
		}

		if imported {
			log.Println("found completed transfer ready for cleanup:", transfer.ID)
			completedTransferIDs = append(completedTransferIDs, transfer.ID)
		}
	}

	if len(completedTransferIDs) > 0 {
		err = j.putioProxy.RemoveTransfers(ctx, true, completedTransferIDs...)
		if err != nil {
			return completedTransferIDs, err
		}
		if j.downloader != nil {
			for _, id := range completedTransferIDs {
				err = j.downloader.RemoveFiles(id)
				if err != nil {
					return completedTransferIDs, fmt.Errorf("failed to remove files for transfer ID `%d`: %w", id, err)
				}
			}
		}
	}

	return completedTransferIDs, nil
}
