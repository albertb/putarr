package fakes

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/putdotio/go-putio"
)

// FakePutio is a minimal, in-memory implementation of a Put.io server.
type FakePutio struct {
	server         *httptest.Server
	configs        map[string]*putioConfigValue
	fileID         int64
	files          map[int64]*putioFile
	deletedFileIDs []int64
	transferID     int64
	transfers      map[int64]*putioTransfer
	zipID          int64
	zips           map[int64]putio.Zip
}

type putioConfigValue struct {
	Value *json.RawMessage
}

type putioFile struct {
	Files  []*putio.File
	Parent putio.File
	Cursor string
}

// putioTransfer wraps putio.Transfer in order to override how time.Time fields are marshaled into JSON since the Put.io
// client expects a specific format.
type putioTransfer struct {
	putio.Transfer
	CreatedAt  *putioTime `json:"created_at"`
	FinishedAt *putioTime `json:"finished_at"`
}

// putioTime is a replacement for putio.Time with the custom JSON marshalling expected by the client.
type putioTime struct {
	Time time.Time
}

func (t putioTime) MarshalJSON() ([]byte, error) {
	return []byte(t.Time.Format(`"2006-01-02 15:04:05"`)), nil
}

func NewFakePutio() *FakePutio {
	rootFolder := putioFile{
		Parent: putio.File{ID: 0, Name: "Your files", ContentType: "application/x-directory", ParentID: -1},
	}

	fake := FakePutio{
		configs:   map[string]*putioConfigValue{},
		files:     map[int64]*putioFile{0: &rootFolder},
		transfers: map[int64]*putioTransfer{},
	}

	mux := http.NewServeMux()

	mux.Handle("GET /v2/config/{key}", handleJSONRPC(func(r *http.Request) (*putioConfigValue, error) {
		key := r.PathValue("key")
		if key == "" {
			return nil, errors.New("missing key in URL")
		}
		val, ok := fake.configs[key]
		if !ok {
			return val, nil
		}
		return val, nil
	}))

	mux.Handle("PUT /v2/config/{key}", handleJSONRPC(func(r *http.Request) (any, error) {
		key := r.PathValue("key")
		if key == "" {
			return nil, errors.New("missing key in URL")
		}
		var val putioConfigValue
		err := json.NewDecoder(r.Body).Decode(&val)
		if err != nil {
			return nil, err
		}
		fake.configs[key] = &val
		return nil, nil
	}))

	mux.Handle("GET /v2/files/list", handleJSONRPC(func(r *http.Request) (putioFile, error) {
		var result putioFile
		values := r.URL.Query()
		id, ok := values["parent_id"]
		if !ok {
			return result, errors.New("missing parent_id in URL")
		}

		parentID, err := strconv.ParseInt(id[0], 10, 64)
		if err != nil {
			return result, fmt.Errorf("invalid parent_id format: %w", err)
		}

		file, ok := fake.files[parentID]
		if !ok {
			return result, fmt.Errorf("unknown file: %d", parentID)
		}

		result = *file
		return result, nil
	}))

	mux.Handle("POST /v2/files/create-folder", handleJSONRPC(func(r *http.Request) (putioFile, error) {
		var result putioFile

		err := r.ParseForm()
		if err != nil {
			return result, fmt.Errorf("failed to parse form: %w", err)
		}

		name := r.FormValue("name")
		if name == "" {
			return result, errors.New("missing name form value")
		}
		id := r.FormValue("parent_id")
		if id == "" {
			return result, errors.New("missing parent_id form value")
		}

		parentID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return result, fmt.Errorf("failed to parse parent_id: %w", err)
		}

		return fake.createFolder(parentID, name)
	}))

	mux.Handle("POST /v2/files/delete", handleJSONRPC(func(r *http.Request) (any, error) {
		err := r.ParseForm()
		if err != nil {
			return nil, fmt.Errorf("failed to parse form: %w", err)
		}

		allFileIDs := r.FormValue("file_ids")
		if allFileIDs == "" {
			return nil, errors.New("missing file_ids form value")
		}

		fileIDs := strings.Split(allFileIDs, ",")
		for _, fileID := range fileIDs {
			id, err := strconv.ParseInt(fileID, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse file ID `%s`: %w", fileID, err)
			}
			fake.deletedFileIDs = append(fake.deletedFileIDs, id)
		}

		return nil, nil
	}))

	type transferList struct{ Transfers []putioTransfer }
	mux.Handle("GET /v2/transfers/list", handleJSONRPC(func(r *http.Request) (transferList, error) {
		var result transferList
		for _, transfer := range fake.transfers {
			result.Transfers = append(result.Transfers, *transfer)
		}
		return result, nil
	}))

	type transferAdd struct{ Transfer putioTransfer }
	mux.Handle("POST /v2/transfers/add", handleJSONRPC(func(r *http.Request) (transferAdd, error) {
		var result transferAdd

		if err := r.ParseForm(); err != nil {
			return result, err
		}

		link := r.FormValue("url")
		callbackURL := r.FormValue("callback_url")

		magnet, err := url.Parse(link)
		if err != nil {
			return result, err
		}

		name := magnet.Query().Get("dn")

		var length int
		if xl := magnet.Query().Get("xl"); xl != "" {
			length, err = strconv.Atoi(xl)
			if err != nil {
				log.Println("failed to parse length from magnet:", err)
			}
		}

		result.Transfer = putioTransfer{
			Transfer: putio.Transfer{
				ID:          atomic.AddInt64(&fake.transferID, 1),
				Name:        name,
				Size:        length,
				PercentDone: 0,
				MagnetURI:   link,
				Status:      "DOWNLOADING",
				CallbackURL: callbackURL,
			},
			CreatedAt: &putioTime{Time: time.Now()},
		}
		fake.transfers[result.Transfer.ID] = &result.Transfer

		return result, nil
	}))

	type transferGet struct{ Transfer putioTransfer }
	mux.Handle("GET /v2/transfers/{id}", handleJSONRPC(func(r *http.Request) (transferGet, error) {
		var result transferGet
		id := r.PathValue("id")
		if id == "" {
			return result, errors.New("missing key in URL")
		}
		transferID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return result, fmt.Errorf("failed to parse ID path value in URL: %w", err)
		}
		transfer, ok := fake.transfers[transferID]
		if !ok {
			return result, fmt.Errorf("transfer with ID `%d` not found", transferID)
		}
		result.Transfer = *transfer
		return result, nil
	}))

	mux.Handle("POST /v2/transfers/cancel", handleJSONRPC(func(r *http.Request) (any, error) {
		err := r.ParseForm()
		if err != nil {
			return nil, err
		}
		idsVal := r.FormValue("transfer_ids")
		ids := strings.Split(idsVal, ",")
		for _, idStr := range ids {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse transfer ID `%s`: %w", idStr, err)
			}
			if _, ok := fake.transfers[id]; !ok {
				return nil, fmt.Errorf("transfer ID not found `%d`", id)
			}
			delete(fake.transfers, id)
		}
		return nil, nil
	}))

	type zipCreate struct {
		ID int64 `json:"zip_id"`
	}
	mux.Handle("POST /v2/zips/create", handleJSONRPC(func(r *http.Request) (zipCreate, error) {
		var result zipCreate
		err := r.ParseForm()
		if err != nil {
			return result, err
		}
		idsVal := r.FormValue("file_ids")
		ids := strings.Split(idsVal, ",")
		for _, id := range ids {
			fileID, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				return result, err
			}
			log.Println(fileID) // TODO
		}
		result.ID = atomic.AndInt64(&fake.zipID, 1)
		return result, nil
	}))

	fake.server = httptest.NewServer(mux)
	return &fake
}

func (s *FakePutio) Close() {
	s.server.Close()
}

func (s *FakePutio) NewClient() *putio.Client {
	putioClient := putio.NewClient(http.DefaultClient)
	serverURL, _ := url.Parse(s.server.URL)
	putioClient.BaseURL = serverURL
	return putioClient
}

func (s *FakePutio) createFolder(parentID int64, name string) (putioFile, error) {
	var folder putioFile
	parent, ok := s.files[parentID]
	if !ok {
		return folder, fmt.Errorf("file with ID %v not found", parentID)
	}

	folder = putioFile{
		Parent: putio.File{
			ID:          atomic.AddInt64(&s.fileID, 1),
			ParentID:    parentID,
			Name:        name,
			ContentType: "application/x-directory",
		},
	}

	parent.Files = append(parent.Files, &folder.Parent)
	s.files[folder.Parent.ID] = &folder

	return folder, nil
}

func (s *FakePutio) CreateFolder(parentID int64, name string) (putio.File, error) {
	folder, err := s.createFolder(parentID, name)
	if err != nil {
		return putio.File{}, err
	}
	return folder.Parent, nil
}

// SetTransferCompleted marks the transfer with the given ID as completed, gives it a file ID, and returns it.
func (s *FakePutio) SetTransferCompleted(id int64) (int64, error) {
	transfer, ok := s.transfers[id]
	if !ok {
		return 0, errors.New("unknown transfer ID")
	}
	transfer.FinishedAt = &putioTime{Time: time.Now()}
	transfer.PercentDone = 100
	transfer.Status = "COMPLETED"
	transfer.FileID = atomic.AddInt64(&s.fileID, 1)
	return transfer.FileID, nil
}

func (s *FakePutio) GetAllDeletedFileIDs() []int64 {
	return s.deletedFileIDs
}
