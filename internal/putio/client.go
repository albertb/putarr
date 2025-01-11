package putio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/albertb/putarr/internal/config"
	"github.com/putdotio/go-putio"
)

// A Put.io transfer, with the added download-dir from Transmission.
type Transfer struct {
	putio.Transfer
	DownloadDir string `json:"download_dir"`
}

// Holds extra state about a Put.io transfer that's required by the Transmission API. Meant to be encoded into the
// transfer's callback URL.
type ExtraState struct {
	DownloadDir string `json:"d"`
}

type Client struct {
	options     *config.Options
	putioClient *putio.Client
}

func NewClient(options *config.Options, putioClient *putio.Client) *Client {
	return &Client{
		options:     options,
		putioClient: putioClient,
	}
}

// Returns the Put.io transfers that this client is responsible for.
func (c *Client) GetTransfers(ctx context.Context) ([]Transfer, error) {
	transfers, err := c.putioClient.Transfers.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []Transfer
	for _, transfer := range transfers {
		if transfer.CallbackURL == "" {
			continue
		}

		extra, err := c.parseCallbackURL(transfer.CallbackURL)
		if err != nil {
			if c.options.Verbose {
				log.Println("failed to parse callbackURL:", err)
			}
			continue
		}

		result = append(result, Transfer{
			Transfer:    transfer,
			DownloadDir: extra.DownloadDir,
		})
	}

	return result, nil
}

func (c *Client) formatCallbackURL(extra *ExtraState) (string, error) {
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
	if len(c.options.Config.Putio.FriendToken) > 0 {
		extraURL.Fragment = c.options.Config.Putio.FriendToken
	}
	return extraURL.String(), nil
}

func (c *Client) parseCallbackURL(callbackURL string) (*ExtraState, error) {
	extraURL, err := url.Parse(callbackURL)
	if err != nil {
		return nil, err
	}

	if (extraURL.Host != "put.test") || extraURL.Path != "/arr" {
		return nil, errors.New("unrecognized callback URL: " + callbackURL)
	}

	if len(c.options.Config.Putio.FriendToken) > 0 {
		if extraURL.Fragment != c.options.Config.Putio.FriendToken {
			return nil, errors.New("the transfer belongs to a different user, fragment=" + extraURL.Fragment)
		}
	}

	params, err := url.ParseQuery(extraURL.RawQuery)
	if err != nil {
		return nil, err
	}
	if len(params["x"]) != 1 {
		return nil, errors.New("unrecognized callback URL: " + callbackURL)
	}

	var extra ExtraState
	err = json.Unmarshal([]byte(params["x"][0]), &extra)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal extra state from callback URL: %w", err)
	}
	return &extra, nil
}

// Adds a new transfer on Put.io for the specified URL (magnet), using the specified download dir.
func (c *Client) AddTransfer(ctx context.Context, url string, dir *string) (*Transfer, error) {
	parent, path, err := c.resolveParentAndPath(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Put.io parent and path: %w", err)
	}

	callbackURL, err := c.formatCallbackURL(&ExtraState{DownloadDir: path})
	if err != nil {
		return nil, fmt.Errorf("failed to format callback URL: %w", err)
	}

	t, err := c.putioClient.Transfers.Add(ctx, url, parent, callbackURL)
	if err != nil {
		return nil, fmt.Errorf("failed to add transfer to Put.io: %w", err)
	}

	return &Transfer{
		Transfer:    t,
		DownloadDir: path,
	}, nil
}

func (c *Client) UploadTorrent(ctx context.Context, torrent []byte, dir *string) (*Transfer, error) {
	return nil, errors.New("TODO implement me")
}

func (c *Client) RemoveTransfer(ctx context.Context, alsoDeleteFiles bool, ids []int64) error {
	for _, id := range ids {
		transfer, err := c.putioClient.Transfers.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get transfers from Put.io: %w", err)
		}

		if _, err = c.parseCallbackURL(transfer.CallbackURL); err != nil {
			// This isn't a transfer we're responsible for.
			if c.options.Verbose {
				log.Println("unrecognized transfer:", err)
			}
			continue
		}

		if alsoDeleteFiles && transfer.FileID > 0 {
			err = c.putioClient.Files.Delete(ctx, transfer.FileID)
			if err != nil {
				return fmt.Errorf("failed to delete files from Put.io: %w", err)
			}
		}

		err = c.putioClient.Transfers.Cancel(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to cancel transfer on Put.io: %w", err)
		}
	}
	return nil
}

func (c *Client) resolveParentAndPath(ctx context.Context, dir *string) (int64, string, error) {
	parent := c.options.Config.Putio.ParentDirID
	if dir == nil {
		return parent, c.options.Config.Transmission.DownloadDir, nil
	}
	id, err := c.createAndReturnDirID(ctx, *dir)
	return id, *dir, err
}

// Treat path as relative to the configured download path. Create the missing sub-directories if necessary and returns
// the ID of the final directory in the full path.
func (c *Client) createAndReturnDirID(ctx context.Context, path string) (int64, error) {
	dir := c.options.Config.Putio.ParentDirID

	subpath := strings.TrimPrefix(path, c.options.Config.Transmission.DownloadDir)
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
		children, _, err := c.putioClient.Files.List(ctx, dir)
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
		created, err := c.putioClient.Files.CreateFolder(ctx, part, dir)
		if err != nil {
			return dir, fmt.Errorf("failed to create folder on Put.io: %w", err)
		}
		dir = created.ID
	}
	return dir, nil
}
