package fakes

type FakeDownloader struct {
	scheduled map[int64]bool
	finished  map[int64]bool
	cleanedUp map[int64]bool
}

func NewFakeDownloader() *FakeDownloader {
	return &FakeDownloader{
		scheduled: make(map[int64]bool),
		finished:  make(map[int64]bool),
		cleanedUp: make(map[int64]bool),
	}
}

// MarkDownloadFinished marks a transfer as finished. By default, transfers that haven't been marked, are not finished.
func (f *FakeDownloader) MarkDownloadFinished(transferID int64) {
	f.finished[transferID] = true
}

// DownloadFilesWereRemoved returns true if the download was cleaned up.
func (f *FakeDownloader) DownloadFilesWereRemoved(transferID int64) bool {
	return f.cleanedUp[transferID]
}

// Implementation of the Downloader interface.
func (f *FakeDownloader) ScheduleDownload(transferID int64, dir string) {
	f.scheduled[transferID] = true
}

func (f *FakeDownloader) IsDownloadFinished(transferID int64) bool {
	return f.finished[transferID]
}

func (f *FakeDownloader) RemoveFiles(transferID int64) error {
	f.cleanedUp[transferID] = true
	return nil
}
