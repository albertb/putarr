package internal

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/go-getter"
	"github.com/putdotio/go-putio"
)

type Downloader interface {
	ScheduleDownload(transferID int64, dir string)
	IsDownloadFinished(transferID int64) bool
	RemoveFiles(transferID int64) error
}

type GoGetterDownloader struct {
	config      *Config
	putioClient *putio.Client

	mu        sync.Mutex
	downloads map[int64]*Download
}

type Download struct {
	path     string             // The path where the downloaded files are saved.
	status   DownloadStatus     // The status of the download.
	cancelFn context.CancelFunc // A function that cancels the download.
}

type DownloadStatus int

const (
	Pending DownloadStatus = iota
	Downloading
	Finished
	Failed
)

func NewDownloader(config *Config, putioClient *putio.Client) *GoGetterDownloader {
	return &GoGetterDownloader{
		config:      config,
		putioClient: putioClient,
		downloads:   make(map[int64]*Download),
	}
}

func (d *GoGetterDownloader) ScheduleDownload(transferID int64, subpath string) {
	path := filepath.Join(d.config.Downloader.Dir, subpath)

	d.mu.Lock()
	defer d.mu.Unlock()

	status, ok := d.downloads[transferID]
	if !ok {
		status = &Download{
			path:   path,
			status: Pending,
		}
		d.downloads[transferID] = status
	}

	// If the download is already in-progress finished, do nothing.
	if status.status == Downloading || status.status == Finished {
		return
	}

	go func() {
		ctx, cancel := context.WithCancel(context.Background())

		d.mu.Lock()
		d.downloads[transferID].status = Downloading
		d.downloads[transferID].cancelFn = cancel
		d.mu.Unlock()

		log.Println("scheduled download; transfer ID:", transferID, "to path:", path)
		err := d.downloadFiles(ctx, path, transferID)

		d.mu.Lock()
		d.downloads[transferID].cancelFn = nil
		defer d.mu.Unlock()

		if err != nil {
			log.Println("failed to download transfer:", err)
			d.downloads[transferID].status = Failed
		} else {
			log.Printf("download finished; transfer ID: %d; path: %s\n", transferID, path)
			d.downloads[transferID].status = Finished
		}
	}()
}

func (d *GoGetterDownloader) downloadFiles(ctx context.Context, path string, transferID int64) error {
	zipID, err := d.getZipID(ctx, transferID)
	if err != nil {
		return err
	}
	url, err := d.getDownloadURL(ctx, zipID)
	if err != nil {
		return err
	}
	if err := getter.Get(path, url, getter.WithContext(ctx)); err != nil {
		return fmt.Errorf("failed to download files: %w", err)
	}
	return nil
}

func (d *GoGetterDownloader) getZipID(ctx context.Context, transferID int64) (int64, error) {
	var zipID int64
	transfer, err := d.putioClient.Transfers.Get(ctx, transferID)
	if err != nil {
		return zipID, fmt.Errorf("failed to get transfer: %w", err)
	}

	zipID, err = d.putioClient.Zips.Create(ctx, transfer.FileID)
	if err != nil {
		return zipID, fmt.Errorf("failed to create zip for transfer: %w", err)
	}
	return zipID, nil
}

func (d *GoGetterDownloader) getDownloadURL(ctx context.Context, zipID int64) (string, error) {
	var url string
	for {
		attempt := 1

		zip, err := d.putioClient.Zips.Get(ctx, zipID)
		if err != nil {
			return url, fmt.Errorf("failed to get zip: %w", err)
		}

		if zip.URL != "" {
			url = zip.URL
			return url, nil
		}

		// Give up after 10 attemps ~= 8 minutes of trying.
		if attempt > 10 {
			return url, fmt.Errorf("failed to get zip with ID %d: no URL after 10 attempts", zipID)
		}

		attempt++
		time.Sleep(time.Duration(2*attempt) * time.Second)
	}
}

func (d *GoGetterDownloader) IsDownloadFinished(transferID int64) bool {
	status, ok := d.downloads[transferID]
	if !ok {
		return false
	}
	return status.status == Finished
}

func (d *GoGetterDownloader) RemoveFiles(transferID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var dir string
	status, ok := d.downloads[transferID]

	if ok {
		if status.cancelFn != nil {
			status.cancelFn() // Cancel the in-progress download if necessary.
		}
		dir = status.path
	} else {
		// If we don't know about this transfer, attempt to manually find the directory using the transfer ID.
		name := strconv.FormatInt(transferID, 10)
		err := filepath.Walk(d.config.Downloader.Dir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && info.Name() == name {
				dir = path
				return nil
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to find downloaded files: %w", err)
		}
	}

	if dir == "" {
		return fmt.Errorf("directory not found; parent `%s`, transfer ID `%d`", d.config.Downloader.Dir, transferID)
	}

	err := os.RemoveAll(dir)
	if err != nil {
		return fmt.Errorf("failed to remove downloaded files: %w", err)
	}

	delete(d.downloads, transferID)
	return nil
}
