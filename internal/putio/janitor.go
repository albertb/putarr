package putio

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/albertb/putarr/internal/arr"
	"github.com/albertb/putarr/internal/config"
	"github.com/putdotio/go-putio"
)

// TODO use my putio Client to do gets and dels

type Janitor struct {
	options     *config.Options
	putioClient *Client
	arrClient   *arr.Client
}

func NewJanitor(options *config.Options, putioClient *Client, arrClient *arr.Client) *Janitor {
	return &Janitor{
		options:     options,
		putioClient: putioClient,
		arrClient:   arrClient,
	}
}

func (j *Janitor) Run() {
	ticker := time.Tick(j.options.Config.Putio.JanitorIntervalOrDefault())
	j.cleanup()
	go func() {
		for range ticker {
			j.cleanup()
		}
	}()
}

func (j *Janitor) cleanup() {
	ctx := context.Background()

	transfers, err := j.getTransfersReadyForCleanup(ctx)
	if err != nil {
		log.Println("failed to get the transfers that are ready for cleanup:", err)
		return
	}

	for _, transfer := range transfers {
		if transfer.FileID != 0 {
			err = j.putioClient.putioClient.Files.Delete(ctx, transfer.FileID)
			if err != nil {
				log.Println("failed to delete file from Put.io:", err)
				continue
			}
		}
		err = j.putioClient.putioClient.Transfers.Cancel(ctx, transfer.ID)
		if err != nil {
			log.Println("failed to cancel transfer on Put.io:", err)
			continue
		}
		if j.options.Verbose {
			log.Println("cleaned up transfer:", transfer.ID, "name:", transfer.Name)
		}
	}
}

func (j *Janitor) getTransfersReadyForCleanup(ctx context.Context) ([]putio.Transfer, error) {
	transfers, err := j.getTransfersByID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get list of transfers from Put.io: %w", err)
	}

	radarrStatus, err := j.arrClient.GetRadarrImportStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get imports from Radarr: %w", err)
	}

	sonarrStatus, err := j.arrClient.GetSonarrImportStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get imports from Sonarr: %w", err)
	}

	var result []putio.Transfer
	for _, transfer := range transfers {
		if status, ok := radarrStatus[transfer.ID]; ok {
			if !status.IsDone() {
				continue
			}
		}
		if status, ok := sonarrStatus[transfer.ID]; ok {
			if !status.IsDone() {
				continue
			}

		}
		result = append(result, transfer)
	}
	return result, nil
}

func (j *Janitor) getTransfersByID(ctx context.Context) (map[int64]putio.Transfer, error) {
	transfers, err := j.putioClient.putioClient.Transfers.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfers from Put.io: %w", err)
	}

	result := make(map[int64]putio.Transfer)
	for _, transfer := range transfers {
		if _, err := j.putioClient.parseCallbackURL(transfer.CallbackURL); err != nil {
			if j.options.Verbose {
				log.Println("unrecognized transfer", transfer.ID, transfer.Name)
			}
			continue // Ignore transfers we're not responsible for.
		}
		result[transfer.ID] = transfer
	}
	return result, nil
}
