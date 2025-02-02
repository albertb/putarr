package fakes

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"golift.io/starr"
	"golift.io/starr/radarr"
	"golift.io/starr/sonarr"
)

// FakeArrs is a minimal, in-memory implementation of a Radarr and Sonarr server.
type FakeArrs struct {
	server *httptest.Server

	radarrQueueID   int64
	radarrQueue     map[int64]*radarr.QueueRecord
	radarrHistoryID int64
	radarrHistory   map[int64]*radarr.HistoryRecord

	sonarrQueueID   int64
	sonarrQueue     map[int64]*sonarr.QueueRecord
	sonarrHistoryID int64
	sonarrHistory   map[int64]*sonarr.HistoryRecord
}

func NewFakeArrs() *FakeArrs {
	fake := FakeArrs{
		radarrQueue:   map[int64]*radarr.QueueRecord{},
		radarrHistory: map[int64]*radarr.HistoryRecord{},
		sonarrQueue:   map[int64]*sonarr.QueueRecord{},
		sonarrHistory: map[int64]*sonarr.HistoryRecord{},
	}

	mux := http.NewServeMux()

	mux.Handle("GET /radarr/api/v3/queue", handleJSONRPC(func(r *http.Request) (radarr.Queue, error) {
		var result radarr.Queue
		for _, record := range fake.radarrQueue {
			result.Records = append(result.Records, record)
		}
		return result, nil
	}))

	mux.Handle("GET /radarr/api/v3/history", handleJSONRPC(func(r *http.Request) (radarr.History, error) {
		var result radarr.History
		for _, record := range fake.radarrHistory {
			result.Records = append(result.Records, record)
		}
		return result, nil
	}))

	mux.Handle("GET /sonarr/api/v3/queue", handleJSONRPC(func(r *http.Request) (sonarr.Queue, error) {
		var result sonarr.Queue
		for _, record := range fake.sonarrQueue {
			result.Records = append(result.Records, record)
		}
		return result, nil
	}))

	mux.Handle("GET /sonarr/api/v3/history", handleJSONRPC(func(r *http.Request) (sonarr.History, error) {
		var result sonarr.History
		for _, record := range fake.sonarrHistory {
			result.Records = append(result.Records, record)
		}
		return result, nil
	}))

	fake.server = httptest.NewServer(mux)
	return &fake
}

func (r *FakeArrs) NewRadarrClient() *radarr.Radarr {
	config := starr.New("whatever", r.server.URL+"/radarr", 0)
	return radarr.New(config)
}

func (r *FakeArrs) NewSonarrClient() *sonarr.Sonarr {
	config := starr.New("whatever", r.server.URL+"/sonarr", 0)
	return sonarr.New(config)
}

func (r *FakeArrs) Close() {
	r.server.Close()
}

func (r *FakeArrs) AddRadarrQueueRecord(record radarr.QueueRecord) int64 {
	record.ID = atomic.AddInt64(&r.radarrQueueID, 1)
	r.radarrQueue[record.ID] = &record
	return record.ID
}

func (r *FakeArrs) RemoveRadarrQueueRecord(id int64) {
	delete(r.radarrQueue, id)
}

func (r *FakeArrs) AddRadarrHistoryRecord(record radarr.HistoryRecord) int64 {
	record.ID = atomic.AddInt64(&r.radarrHistoryID, 1)
	r.radarrHistory[record.ID] = &record
	return record.ID
}

func (r *FakeArrs) AddSonarrQueueRecord(record sonarr.QueueRecord) int64 {
	record.ID = atomic.AddInt64(&r.sonarrQueueID, 1)
	r.sonarrQueue[record.ID] = &record
	return record.ID
}

func (r *FakeArrs) RemoveSonarrQueueRecord(id int64) {
	delete(r.sonarrQueue, id)
}

func (r *FakeArrs) AddSonarrHistoryRecord(record sonarr.HistoryRecord) int64 {
	record.ID = atomic.AddInt64(&r.sonarrHistoryID, 1)
	r.sonarrHistory[record.ID] = &record
	return record.ID
}
