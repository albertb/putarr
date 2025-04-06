package internal

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/albertb/putarr/internal/fakes"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackpal/bencode-go"
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func TestTransmissionRPC_AuthAndSessionMiddleware(t *testing.T) {
	var (
		username = "azure"
		password = "hunter2"
		token    = "whatever"
	)

	config := &Config{
		Transmission: TransmissionConfig{
			Username: username,
			Password: password,
		}}

	fakePutio := fakes.NewFakePutio()
	defer fakePutio.Close()

	server := httptest.NewServer(NewServer(config, token,
		NewPutioProxy(config, fakePutio.NewClient())))
	defer server.Close()

	for _, tt := range []struct {
		explanation string
		method      string
		username    string
		password    string
		token       string
		status      int
	}{
		{
			"requests without basic auth are unauthorized",
			"GET",
			"",
			"",
			"",
			http.StatusUnauthorized,
		},
		{
			"requests with the wrong credentials are unauthorized",
			"GET",
			"whatever",
			"wrongpassword",
			"",
			http.StatusUnauthorized,
		},
		{
			"requests with the correct credentials but no session token fail with a conflict",
			"GET",
			username,
			password,
			"",
			http.StatusConflict,
		},
		{
			"requests with the correct credentials but the wrong session token fail with a conflict",
			"GET",
			username,
			password,
			"wrongtoken",
			http.StatusConflict,
		},
		{
			"requests with the correct credentials and session token are successful",
			"GET",
			username,
			password,
			token,
			http.StatusOK,
		},
		{
			"requests without basic auth are unauthorized",
			"POST",
			"",
			"",
			"",
			http.StatusUnauthorized,
		},
		{
			"requests with the wrong credentials are unauthorized",
			"POST",
			"whatever",
			"wrongpassword",
			"",
			http.StatusUnauthorized,
		},
		{
			"requests with the correct credentials but no session token fail with a conflict",
			"POST",
			username,
			password,
			"",
			http.StatusConflict,
		},
		{
			"requests with the correct credentials but the wrong session token fail with a conflict",
			"POST",
			username,
			password,
			"wrongtoken",
			http.StatusConflict,
		},
		{
			"requests with the correct credentials and session token but no JSON fail with bad request",
			"POST",
			username,
			password,
			token,
			http.StatusBadRequest,
		},
	} {
		t.Run(fmt.Sprintf("%s [%s]", tt.explanation, tt.method), func(t *testing.T) {
			req, err := http.NewRequest(tt.method, server.URL+"/transmission/rpc", nil)
			if err != nil {
				t.Fatal(err)
			}

			if tt.username != "" && tt.password != "" {
				req.SetBasicAuth(tt.username, tt.password)
			}

			if tt.token != "" {
				req.Header.Add("X-Transmission-Session-ID", tt.token)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if got, want := resp.StatusCode, tt.status; got != want {
				t.Fatalf("unexpected status code. got `%v`, want `%v`", got, want)
			}
		})
	}
}

func TestTransmissionRPC_GetSession(t *testing.T) {
	var (
		username    = "azure"
		password    = "hunter2"
		token       = "whatever"
		downloadDir = "/putarr"
	)

	config := &Config{
		Transmission: TransmissionConfig{
			Username:    username,
			Password:    password,
			DownloadDir: downloadDir,
		}}

	fakePutio := fakes.NewFakePutio()
	defer fakePutio.Close()

	server := httptest.NewServer(NewServer(config, token,
		NewPutioProxy(config, fakePutio.NewClient())))
	defer server.Close()

	got := doRPCAndExpectOK[Session](t, config, server.URL, token, "session-get", nil)
	want := Session{
		RPCVersion:  "18",
		Version:     "14.0.0",
		DownloadDir: downloadDir,
	}

	if got != want {
		t.Fatalf("unexpected session. got %+v, want %+v", got, want)
	}
}

func TestTransmissionRPC_TorrentAddWithBadDir(t *testing.T) {
	var (
		username    = "azure"
		password    = "hunter2"
		token       = "whatever"
		downloadDir = "/putarr"
	)

	config := &Config{
		Transmission: TransmissionConfig{
			Username:    username,
			Password:    password,
			DownloadDir: downloadDir,
		}}

	fakePutio := fakes.NewFakePutio()
	defer fakePutio.Close()

	folder, err := fakePutio.CreateFolder(0, "putarr")
	if err != nil {
		t.Fatalf("failed to create new Put.io folder: %s", err)
	}
	config.Putio.ParentDirID = folder.ID

	server := httptest.NewServer(NewServer(config, token,
		NewPutioProxy(config, fakePutio.NewClient())))
	defer server.Close()

	// Attempting to start a download with a download-dir that isn't a child of the configured download-dir should
	// result in a failure.
	doRPCAndExpectCode[Torrent](t, config, server.URL, token, "torrent-add", map[string]any{
		"filename":     "magnet:?xt=urn:btih:AAA&dn=foo",
		"download-dir": "/whatever"},
		http.StatusInternalServerError)
}

func mapTorrentsByID(torrents []Torrent) map[int]*Torrent {
	m := map[int]*Torrent{}
	for _, torrent := range torrents {
		m[torrent.ID] = &torrent
	}
	return m
}

// Ignore element order when comparing []ints with cmp.Equal.
var sliceOpts = cmpopts.SortSlices(func(x, y int) bool {
	return x < y
})

func TestTransmissionRPC_TorrentAddGetRemove(t *testing.T) {
	var (
		username    = "azure"
		password    = "hunter2"
		token       = "whatever"
		downloadDir = "/putarr"
	)

	config := &Config{
		Downloader: DownloaderConfig{Dir: "/download"},
		Transmission: TransmissionConfig{
			Username:    username,
			Password:    password,
			DownloadDir: downloadDir,
		}}

	fakePutio := fakes.NewFakePutio()
	defer fakePutio.Close()

	folder, err := fakePutio.CreateFolder(0, "putarr")
	if err != nil {
		t.Fatalf("failed to create new Put.io folder: %s", err)
	}
	config.Putio.ParentDirID = folder.ID

	server := httptest.NewServer(NewServer(config, token,
		NewPutioProxy(config, fakePutio.NewClient())))
	defer server.Close()

	// Initially the list of torrents is empty.
	torrents := doRPCAndExpectOK[map[string][]Torrent](t, config, server.URL, token, "torrent-get", nil)

	_, ok := torrents["torrents"]
	if got, want := ok, true; got != want {
		t.Fatalf("missing `torrents` key in response")
	}
	if got, want := len(torrents["torrents"]), 0; got != want {
		t.Fatalf("got %d torrents, want %d", got, want)
	}

	// Add three torrents.
	torrent1 := doRPCAndExpectOK[Torrent](t, config, server.URL, token, "torrent-add", map[string]any{
		"filename":     "magnet:?xt=urn:btih:AAA&dn=foo",
		"download-dir": "/putarr/tv-sonarr"})
	torrent2 := doRPCAndExpectOK[Torrent](t, config, server.URL, token, "torrent-add", map[string]any{
		"filename":     "magnet:?xt=urn:btih:BBB&dn=bar",
		"download-dir": "/putarr/tv-sonarr/whatever"})
	torrent3 := doRPCAndExpectOK[Torrent](t, config, server.URL, token, "torrent-add", map[string]any{
		"filename":     "magnet:?xt=urn:btih:CCC&dn=baz",
		"download-dir": "/putarr/tv-sonarr"})

	// We should get a list of three torrents back.
	torrents = doRPCAndExpectOK[map[string][]Torrent](t, config, server.URL, token, "torrent-get", nil)

	if got, want := len(torrents["torrents"]), 3; got != want {
		t.Fatalf("got a list of %v torrents, want %v", got, want)
	}
	torrentsByID := mapTorrentsByID(torrents["torrents"])

	if got, want := slices.Collect(maps.Keys(torrentsByID)), []int{torrent1.ID, torrent2.ID, torrent3.ID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got torrents with IDs %v, want %v", got, want)
	}

	for id, name := range map[int]string{
		torrent1.ID: "foo",
		torrent2.ID: "bar",
		torrent3.ID: "baz"} {
		if torrent, ok := torrentsByID[id]; !ok {
			t.Fatalf("missing torrent with ID %v", id)
		} else {
			if got, want := torrent.Name, name; got != want {
				t.Errorf("got torrent[%d].Name = %v, want %v", id, got, want)
			}
		}
	}
	for id, dir := range map[int]string{
		torrent1.ID: "/putarr/tv-sonarr",
		torrent2.ID: "/putarr/tv-sonarr/whatever",
		torrent3.ID: "/putarr/tv-sonarr"} {
		if torrent, ok := torrentsByID[id]; !ok {
			t.Fatalf("missing torrent with ID %v", id)
		} else {
			if got, want := torrent.DownloadDir, dir; got != want {
				t.Errorf("got torrent[%d].DownloadDir = %v, want %v", id, got, want)
			}
		}
	}

	// Mark the second torrent as completed, and keep track of the file ID that was downloaded.
	fileID, _ := fakePutio.SetTransferCompleted(int64(torrent2.ID))

	// Get the list again, the second torrent should be completed.
	torrents = doRPCAndExpectOK[map[string][]Torrent](t, config, server.URL, token, "torrent-get", nil)

	if got, want := len(torrents["torrents"]), 3; got != want {
		t.Fatalf("got a list of %v torrents, want %v", got, want)
	}
	torrentsByID = mapTorrentsByID(torrents["torrents"])

	// Make sure only the second torrent is finished.
	for id, finished := range map[int]bool{
		torrent1.ID: false,
		torrent2.ID: true,
		torrent3.ID: false} {
		if torrent, ok := torrentsByID[id]; !ok {
			t.Fatalf("missing torrent with ID %v", id)
		} else {
			if got, want := torrent.IsFinished, finished; got != want {
				t.Fatalf("got torrent[%d].IsFinished = %v, want %v", id, got, want)
			}
		}
	}

	// Remove the first torrent.
	doRPCAndExpectOK[any](t, config, server.URL, token, "torrent-remove", map[string]any{
		"delete-local-data": true,
		"ids":               []string{*torrent1.HashString},
	})

	// Get the list again, it should now just have two torrents.
	torrents = doRPCAndExpectOK[map[string][]Torrent](t, config, server.URL, token, "torrent-get", nil)
	torrentsByID = mapTorrentsByID(torrents["torrents"])
	if got, want := slices.Collect(maps.Keys(torrentsByID)), []int{torrent2.ID, torrent3.ID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got torrent IDs %v, want %v", got, want)
	}

	// Remove the last two torrents.
	doRPCAndExpectOK[any](t, config, server.URL, token, "torrent-remove", map[string]any{
		"delete-local-data": true,
		"ids":               []string{*torrent2.HashString, *torrent3.HashString},
	})

	// Get the list again, it should now be empty.
	torrents = doRPCAndExpectOK[map[string][]Torrent](t, config, server.URL, token, "torrent-get", nil)
	if got, want := len(torrents["torrents"]), 0; got != want {
		t.Fatalf("got len(torrents) %v, want %v", got, want)
	}

	// Finally, expect the transfer files to be deleted.
	if got, want := fakePutio.GetAllDeletedFileIDs(), []int64{fileID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got deleted file IDs %v, want %v", got, want)
	}
}

func TestTransmissionRPC_TorrentAddGetWithFriendToken(t *testing.T) {
	var (
		username = "azure"
		password = "hunter2"
		token    = "whatever"
	)

	fakePutio := fakes.NewFakePutio()
	defer fakePutio.Close()

	configA := &Config{
		Transmission: TransmissionConfig{
			Username:    username,
			Password:    password,
			DownloadDir: "/aaa",
		}, Putio: PutioConfig{
			FriendToken: "aaa",
		}}

	folderA, err := fakePutio.CreateFolder(0, "aaa")
	if err != nil {
		t.Fatalf("failed to create new Put.io folder: %s", err)
	}
	configA.Putio.ParentDirID = folderA.ID

	configB := &Config{
		Transmission: TransmissionConfig{
			Username:    username,
			Password:    password,
			DownloadDir: "/bbb",
		}, Putio: PutioConfig{
			FriendToken: "bbb",
		}}

	folderB, err := fakePutio.CreateFolder(0, "bbb")
	if err != nil {
		t.Fatalf("failed to create new Put.io folder: %s", err)
	}
	configB.Putio.ParentDirID = folderB.ID

	// Setup two putarr servers that share the same Put.io account, but use the two different friend tokens.
	serverA := httptest.NewServer(NewServer(configA, token,
		NewPutioProxy(configA, fakePutio.NewClient())))
	defer serverA.Close()

	serverB := httptest.NewServer(NewServer(configB, token,
		NewPutioProxy(configB, fakePutio.NewClient())))
	defer serverB.Close()

	// Initially the list of torrents is empty for both servers.
	torrentsA := doRPCAndExpectOK[map[string][]Torrent](t, configA, serverA.URL, token, "torrent-get", nil)
	if got, want := len(torrentsA["torrents"]), 0; got != want {
		t.Fatalf("got %d torrents, want %d", got, want)
	}
	torrentsB := doRPCAndExpectOK[map[string][]Torrent](t, configB, serverB.URL, token, "torrent-get", nil)
	if got, want := len(torrentsB["torrents"]), 0; got != want {
		t.Fatalf("got %d torrents, want %d", got, want)
	}

	// Add a torrent through serverA.
	torrentA := doRPCAndExpectOK[Torrent](t, configA, serverA.URL, token, "torrent-add", map[string]any{
		"filename":     "magnet:?xt=urn:btih:AAA&dn=foo",
		"download-dir": "/aaa/tv-sonarr"})

	// Make sure serverA lists the new torrent.
	torrentsA = doRPCAndExpectOK[map[string][]Torrent](t, configA, serverA.URL, token, "torrent-get", nil)
	if got, want := slices.Collect(maps.Keys(mapTorrentsByID(torrentsA["torrents"]))), []int{torrentA.ID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got torrents with IDs %v, want %v", got, want)
	}

	// ServerB is still empty.
	torrentsB = doRPCAndExpectOK[map[string][]Torrent](t, configB, serverB.URL, token, "torrent-get", nil)
	if got, want := len(torrentsB["torrents"]), 0; got != want {
		t.Fatalf("got %d torrents, want %d", got, want)
	}

	// Next add a torrent through serverB.
	torrentB := doRPCAndExpectOK[Torrent](t, configB, serverB.URL, token, "torrent-add", map[string]any{
		"filename":     "magnet:?xt=urn:btih:BBB&dn=foo",
		"download-dir": "/bbb/tv-sonarr"})

	// ServerA should only list its own torrent.
	torrentsA = doRPCAndExpectOK[map[string][]Torrent](t, configA, serverA.URL, token, "torrent-get", nil)
	if got, want := slices.Collect(maps.Keys(mapTorrentsByID(torrentsA["torrents"]))), []int{torrentA.ID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got torrents with IDs %v, want %v", got, want)
	}

	// And ServerB should list the new torrent.
	torrentsB = doRPCAndExpectOK[map[string][]Torrent](t, configB, serverB.URL, token, "torrent-get", nil)
	if got, want := slices.Collect(maps.Keys(mapTorrentsByID(torrentsB["torrents"]))), []int{torrentB.ID}; !cmp.Equal(got, want, sliceOpts) {
		t.Fatalf("got torrents with IDs %v, want %v", got, want)
	}
}

type TorrentFile struct {
	Announce string          `bencode:"announce"`
	Info     TorrentFileInfo `bencode:"info"`
}
type TorrentFileInfo struct {
	Name   string `bencode:"name"`
	Length int64  `bencode:"length"`
}

func TestTransmissionRPC_TorrentAddWithTorrentFile(t *testing.T) {
	var (
		username    = "azure"
		password    = "hunter2"
		token       = "whatever"
		downloadDir = "/putarr"
	)

	config := &Config{
		Transmission: TransmissionConfig{
			Username:    username,
			Password:    password,
			DownloadDir: downloadDir,
		}}

	fakePutio := fakes.NewFakePutio()
	defer fakePutio.Close()

	folder, err := fakePutio.CreateFolder(0, "putarr")
	if err != nil {
		t.Fatalf("failed to create new Put.io folder: %s", err)
	}
	config.Putio.ParentDirID = folder.ID

	server := httptest.NewServer(
		NewServer(config, token,
			NewPutioProxy(config, fakePutio.NewClient())))
	defer server.Close()

	// A minimal torrent file.
	torrent := TorrentFile{
		Announce: "example.org/tracker",
		Info: TorrentFileInfo{
			Name:   "example.filename",
			Length: 123456,
		},
	}

	var buf bytes.Buffer
	err = bencode.Marshal(&buf, torrent)
	if err != nil {
		t.Fatalf("failed to marshal torrent: %s", err)
	}

	added := doRPCAndExpectOK[Torrent](t, config, server.URL, token, "torrent-add", map[string]any{
		"metainfo":     base64.StdEncoding.EncodeToString(buf.Bytes()),
		"download-dir": "/putarr/tv-sonarr"})

	// There's not much we can easily test at this level, but at least make sure the name and length of the transfer on
	// Put.io after all the conversions matches the ones in the torrent file.
	if got, want := added.Name, torrent.Info.Name; got != want {
		t.Fatalf("got torrent name %s, want %s", got, want)
	}
	if got, want := added.TotalSize, torrent.Info.Length; got != want {
		t.Fatalf("got torrent length %d, want %d", got, want)
	}
}

func doRPCAndExpectOK[T any](t *testing.T, config *Config, baseURL string, token string, method string, args map[string]any) T {
	t.Helper()
	return doRPCAndExpectCode[T](t, config, baseURL, token, method, args, http.StatusOK)
}

func doRPCAndExpectCode[T any](t *testing.T, config *Config, baseURL string, token string, method string, args map[string]any, code int) T {
	t.Helper()

	request := Request{
		Method:    method,
		Arguments: args,
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(request)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("POST", baseURL+"/transmission/rpc", &buf)
	if err != nil {
		t.Fatal(err)
	}

	req.SetBasicAuth(config.Transmission.Username, config.Transmission.Password)
	req.Header.Add("X-Transmission-Session-ID", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, code; got != want {
		t.Fatalf("unexpected status code. got `%v`, want `%v`", got, want)
	}

	if code != http.StatusOK {
		var v T
		return v
	}

	var response Response
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := response.Result, "success"; got != want {
		t.Fatalf("unexpected response result. got `%v`, want `%v`", response, want)
	}

	var v T
	err = json.Unmarshal(response.Arguments, &v)
	if err != nil {
		t.Fatal(err)
	}

	return v
}
