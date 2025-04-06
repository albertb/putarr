package internal

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/jackpal/bencode-go"
	"github.com/putdotio/go-putio"
)

// Transfer embeds a putio.Transfer and adds the download directory field from the Transmission API.
type Transfer struct {
	*putio.Transfer
	DownloadDir string
}

// PutioProxy proxies Transmission API RPCs to Put.io.
type PutioProxy struct {
	config      *Config
	putioClient *putio.Client
}

func NewPutioProxy(config *Config, putioClient *putio.Client) *PutioProxy {
	return &PutioProxy{
		config:      config,
		putioClient: putioClient,
	}
}

func (p *PutioProxy) AddTransfer(ctx context.Context, magnet, downloadDir string) (Transfer, error) {
	var result Transfer
	parentID, err := p.createAndReturnDirID(ctx, downloadDir)
	if err != nil {
		return result, fmt.Errorf("failed to create download directory on Put.io: %w", err)
	}

	callbackURL, err := p.formatCallbackURL(extraState{DownloadDir: downloadDir})
	if err != nil {
		return result, fmt.Errorf("failed to format callback URL: %w", err)
	}

	transfer, err := p.putioClient.Transfers.Add(ctx, magnet, parentID, callbackURL)
	if err != nil {
		return result, err
	}

	result.Transfer = &transfer
	result.DownloadDir = fmt.Sprintf("%s/%d", downloadDir, transfer.ID)
	return result, nil
}

func (p *PutioProxy) UploadTorrent(ctx context.Context, file []byte, downloadDir string) (Transfer, error) {
	// We could upload the torrent directly to Put.io and it would work just fine, but we want to be able to add a
	// callback URL to the transfer so we can identify it later. The Transfer API lets us add a callback URL, but it
	// requires a magnet link instead of a torrent.
	var transfer Transfer

	// Decode the torrent file and extract a dictionary of its fields.
	decoded, err := bencode.Decode(bytes.NewReader(file))
	if err != nil {
		return transfer, fmt.Errorf("failed to decode torrent file: %w", err)
	}
	torrent, ok := decoded.(map[string]interface{})
	if !ok {
		return transfer, errors.New("unable to parse torrent file")
	}

	// Calculate the info hash of the torrent and use it as the basis for the magnet link.
	var buf bytes.Buffer
	err = bencode.Marshal(&buf, torrent["info"])
	if err != nil {
		return transfer, fmt.Errorf("failed to encode torrent info: %w", err)
	}
	checksum := sha1.Sum(buf.Bytes())
	magnet := "magnet:?xt=urn:btih:" + base32.StdEncoding.EncodeToString(checksum[:])

	// Add the name and length of the torrent to the magnet link, if available.
	if info, ok := torrent["info"].(map[string]interface{}); ok {
		if name, ok := info["name"].(string); ok {
			magnet += "&dn=" + url.QueryEscape(name)
		}
		if length, ok := info["length"].(int64); ok {
			magnet += "&xl=" + fmt.Sprint(length)
		}
	}

	// Add the tracker to the magnet link, if available.
	if tracker, ok := torrent["announce"].(string); ok {
		magnet += "&tr=" + url.QueryEscape(tracker)
	}

	// Add the transfer to Put.io using the Transfer API.
	return p.AddTransfer(ctx, magnet, downloadDir)
}

func (p *PutioProxy) GetTransfers(ctx context.Context) ([]Transfer, error) {
	var result []Transfer
	transfers, err := p.putioClient.Transfers.List(ctx)
	if err != nil {
		return result, err
	}
	for _, transfer := range transfers {
		extra, err := p.parseCallbackURL(transfer.CallbackURL)
		if err != nil {
			log.Println("cannot parse callback URL, skipping transfer:", err)
			continue
		}

		downloadDir := extra.DownloadDir
		result = append(result, Transfer{
			Transfer:    &transfer,
			DownloadDir: downloadDir,
		})
	}
	return result, nil
}

func (p *PutioProxy) RemoveTransfers(ctx context.Context, removeFiles bool, ids ...int64) error {
	for _, id := range ids {
		transfer, err := p.putioClient.Transfers.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get transfer with ID `%d`: %w", id, err)
		}
		_, err = p.parseCallbackURL(transfer.CallbackURL)
		if err != nil {
			log.Println("cannot parse callback URL, skipping transfer:", err)
			continue
		}
		if removeFiles && transfer.FileID != 0 {
			err = p.putioClient.Files.Delete(ctx, transfer.FileID)
			if err != nil {
				return fmt.Errorf("failed to delete file with ID `%d`: %w", transfer.FileID, err)
			}
		}
		err = p.putioClient.Transfers.Cancel(ctx, transfer.ID)
		if err != nil {
			return fmt.Errorf("failed to cancel transfer with ID `%d`: %w", transfer.ID, err)
		}
	}
	return nil
}

// Treat path as relative to the configured download path. Create the missing sub-directories if necessary and returns
// the ID of the final directory in the full path.
func (p *PutioProxy) createAndReturnDirID(ctx context.Context, path string) (int64, error) {
	dir := p.config.Putio.ParentDirID

	subpath := strings.TrimPrefix(path, p.config.Transmission.DownloadDir)
	if subpath == path {
		return dir, fmt.Errorf("download directory must be a subdirectory of `%s`", p.config.Transmission.DownloadDir)
	}

	// If there's no subpath beyond the default download directory, we're done.
	if subpath == "" {
		return dir, nil
	}

	// Trim off the leading slash if present to avoid an empty string when splitting later.
	if strings.HasPrefix(subpath, string(filepath.Separator)) {
		subpath = subpath[len(string(filepath.Separator)):]
	}

	// Split the subpath into individual directories and walk the Put.io tree to create the missing ones.
	parts := strings.Split(subpath, string(filepath.Separator))

	for _, part := range parts {
		children, _, err := p.putioClient.Files.List(ctx, dir)
		if err != nil {
			return dir, fmt.Errorf("failed to list files on Put.io: %w", err)
		}

		// If this directory already exists, move on to the next sub-directory.
		exists := false
		for _, child := range children {
			if child.Name == part && child.IsDir() {
				dir = child.ID
				exists = true
				break
			}
		}
		if exists {
			continue
		}

		// Directory not found; create it before moving on to the next sub-directory.
		created, err := p.putioClient.Files.CreateFolder(ctx, part, dir)
		if err != nil {
			return dir, fmt.Errorf("failed to create folder on Put.io: %w", err)
		}
		dir = created.ID
	}
	return dir, nil
}

// Holds extra state about a Put.io transfer that's required by the Transmission API. Meant to be encoded into the
// transfer's callback URL. In theory, we could instead store this in the Put.io ConfigService, but using the callback
// URL is more convenient since it means this extra state will have the same lifetime as the transfer itself.
type extraState struct {
	DownloadDir string `json:"d"`
}

func (p *PutioProxy) formatCallbackURL(extra extraState) (string, error) {
	data, err := json.Marshal(extra)
	if err != nil {
		return "", err
	}

	// This is probably overkill, but encode the ExtraState into a legit-looking URL.
	params := url.Values{
		"x": {string(data)},
	}
	extraURL := url.URL{
		Scheme:   "test",
		Host:     "put.test", // The .test TLD is garanteed not to resolve.
		Path:     "/arr",
		RawQuery: params.Encode(),
	}
	if len(p.config.Putio.FriendToken) > 0 {
		extraURL.Fragment = p.config.Putio.FriendToken
	}
	return extraURL.String(), nil
}

func (p *PutioProxy) parseCallbackURL(callbackURL string) (extraState, error) {
	var result extraState

	extraURL, err := url.Parse(callbackURL)
	if err != nil {
		return result, err
	}

	if (extraURL.Host != "put.test") || extraURL.Path != "/arr" {
		return result, errors.New("unrecognized callback URL: " + callbackURL)
	}

	if len(p.config.Putio.FriendToken) > 0 {
		if extraURL.Fragment != p.config.Putio.FriendToken {
			return result, errors.New("the transfer belongs to a different user, fragment=" + extraURL.Fragment)
		}
	}

	params, err := url.ParseQuery(extraURL.RawQuery)
	if err != nil {
		return result, err
	}
	if len(params["x"]) != 1 {
		return result, errors.New("unrecognized callback URL: " + callbackURL)
	}

	err = json.Unmarshal([]byte(params["x"][0]), &result)
	if err != nil {
		return result, fmt.Errorf("failed to unmarshal extra state from callback URL: %w", err)
	}
	return result, nil
}
