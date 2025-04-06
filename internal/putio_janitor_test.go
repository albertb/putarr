package internal

import (
	"context"
	"log"
	"testing"

	"github.com/albertb/putarr/internal/fakes"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golift.io/starr/radarr"
	"golift.io/starr/sonarr"
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func TestJanitor(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		Transmission: TransmissionConfig{
			DownloadDir: "/",
		},
	}

	fakePutio := fakes.NewFakePutio()
	defer fakePutio.Close()

	fakeArrs := fakes.NewFakeArrs()
	defer fakeArrs.Close()

	arrClient := NewArrClient(config, fakeArrs.NewRadarrClient(), fakeArrs.NewSonarrClient())
	putioProxy := NewPutioProxy(config, fakePutio.NewClient())

	janitor := NewPutioJanitor(arrClient, putioProxy)

	// Start by adding in-progress transfers for a movie and some episodes.
	movieTransfer, err := putioProxy.AddTransfer(ctx, "magnet:?xt=urn:btih:AAA&dn=movie", "/")
	if err != nil {
		t.Fatalf("failed to add movie transfer: %s", err)
	}

	// Add a corresponding queue records: the movie and show imports are blocked on their transfers completing.
	movieQueueID := fakeArrs.AddRadarrQueueRecord(
		radarr.QueueRecord{MovieID: 123, DownloadID: FormatTorrentHash(movieTransfer.ID)})

	// Next, add an in-progress transfer for some episodes.
	showTransfer, err := putioProxy.AddTransfer(ctx, "magnet:?xt=urn:btih:BBB&dn=episodes", "/")
	if err != nil {
		t.Fatalf("failed to add show transfer: %s", err)
	}

	// Add corresponding queue records: none of the episodes are been imported yet.
	showQueueIDs := []int64{
		fakeArrs.AddSonarrQueueRecord(sonarr.QueueRecord{EpisodeID: 100, DownloadID: FormatTorrentHash(showTransfer.ID)}),
		fakeArrs.AddSonarrQueueRecord(sonarr.QueueRecord{EpisodeID: 101, DownloadID: FormatTorrentHash(showTransfer.ID)}),
		fakeArrs.AddSonarrQueueRecord(sonarr.QueueRecord{EpisodeID: 103, DownloadID: FormatTorrentHash(showTransfer.ID)}),
	}

	// The janitor shouldn't find any transfers to cleanup.
	ids, err := janitor.RunOnce(ctx)
	if err != nil {
		t.Fatalf("failed to run janitor: %s", err)
	}
	if got, want := len(ids), 0; got != want {
		t.Fatalf("got len(cleaned up transfers) %d, want %d", got, want)
	}

	// The movie import is complete: mark the transfer as completed, remove the queue record, and add a history record.
	movieFileID, _ := fakePutio.SetTransferCompleted(movieTransfer.ID)
	fakeArrs.RemoveRadarrQueueRecord(movieQueueID)
	fakeArrs.AddRadarrHistoryRecord(radarr.HistoryRecord{MovieID: 123, DownloadID: FormatTorrentHash(movieTransfer.ID)})

	ids, err = janitor.RunOnce(ctx)
	if err != nil {
		t.Fatalf("failed to run janitor: %s", err)
	}

	// Ignore element order when comparing []int64.
	sliceOpts := cmpopts.SortSlices(func(x, y int64) bool {
		return x < y
	})

	// We expect the movie transfer to get cleaned up now since the import is complete.
	if got, want := ids, []int64{movieTransfer.ID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got %v cleaned up transfers, want %v", got, want)
	}

	// We also expect the movie file to have been deleted.
	if got, want := fakePutio.GetAllDeletedFileIDs(), []int64{movieFileID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got %v deleted file IDs, want %v", got, want)
	}

	// The show transfer is complete, and some but not all of the imports are done.
	showFileID, _ := fakePutio.SetTransferCompleted(showTransfer.ID)
	fakeArrs.RemoveSonarrQueueRecord(showQueueIDs[0])
	fakeArrs.AddSonarrHistoryRecord(sonarr.HistoryRecord{EpisodeID: 100, DownloadID: FormatTorrentHash(showTransfer.ID)})

	ids, err = janitor.RunOnce(ctx)
	if err != nil {
		t.Fatalf("failed to run janitor: %s", err)
	}

	// We don't expect the show transfer to get cleaned up yet because not all episodes have been imported.
	if got, want := len(ids), 0; got != want {
		t.Fatalf("got len(cleanep up transfers) %d, want %d", got, want)
	}

	// Mark the missing episode imports as completed. All of the imports are now done.
	fakeArrs.RemoveSonarrQueueRecord(showQueueIDs[1])
	fakeArrs.AddSonarrHistoryRecord(sonarr.HistoryRecord{EpisodeID: 101, DownloadID: FormatTorrentHash(showTransfer.ID)})
	fakeArrs.RemoveSonarrQueueRecord(showQueueIDs[2])
	fakeArrs.AddSonarrHistoryRecord(sonarr.HistoryRecord{EpisodeID: 102, DownloadID: FormatTorrentHash(showTransfer.ID)})

	ids, err = janitor.RunOnce(ctx)
	if err != nil {
		t.Fatalf("failed to run janitor: %s", err)
	}

	// Expect the show transfer to get cleaned up now.
	if got, want := ids, []int64{showTransfer.ID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got cleanup up transfers %v, want %v", got, want)
	}

	// Expect both the movie and the show files to have been deleted.
	if got, want := fakePutio.GetAllDeletedFileIDs(), []int64{movieFileID, showFileID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got deleted files %v, want %v", got, want)
	}
}
