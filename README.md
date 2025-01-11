# Putarr

Putarr is a tool to manage and automate your media downloads from Put.io, integrating with Radarr and Sonarr.

## Features

- **Put.io Integration**: Manage your Put.io transfers and files.
- **Radarr and Sonarr Integration**: Automatically import movies and TV shows.
- **Transmission RPC**: Emulate Transmission RPC for compatibility for ease of integration with *arrs.
- **Janitor Service**: Automatically clean up completed transfers on Put.io.

## Installation

### Docker

#### Build

```sh
docker build -t docker-putarr -f docker/Dockerfile .
```

#### Run

```sh
docker run -d -v $HOME/.config/putarr:/config -p 9091:9091 --name putarr docker-putarr -v
```

## Configuration

Create a configuration file at `$HOME/.config/putarr/putarr.yaml` with the following structure:

```yaml
transmission:
    # This is the username/password that clients (e.g., Radarr/Sonarr) need to provide in
    # order to communicate with Putarr.
    username: your_username
    password: your_password

    # The path where downloads are available from the point of view of Radarr/Sonarr. This is
    # most likely the path where you've mounted your Put.io account using rclone, see later
    # sections.
    download_dir: /path/to/download

putio:
    # OAuth token to communicate with Put.io.
    oauth_token: your_oauth_token

    # The ID of the parent directory where to store downloaded files. When this is unset files 
    # are saved in the root. When this is -1, the default directory configured in the Put.io
    # account is used. You can find the ID of a directory in the URL when browing your files
    # on Put.io.
    parent_dir_id: your_parent_dir_id

    # How often to clean up the transfers and files of successfully imported media.
    janitor_interval: 1h

    # The friend token used to identify the transfers belonging to this instance of Putarr.
    friend_token: friend_token

radarr:
    url: http://localhost:7878
    api_key: your_radarr_api_key

sonarr:
    url: http://localhost:8989
    api_key: your_sonarr_api_key
```

### Download client

To use Putarr from Radarr/Sonarr, configure a new Transmission client, with the same username
and password as in the Putarr config file, and host/port where Putarr is running.

# Rclone

Note that Putarr only directs Put.io to download files, it doesn't make those files available
locally. To make it possible to import the downloaded files, a simple solution is to use
[`rclone`](http://rclone.org/putio) to mount your Put.io account as a local filesystem.

For example, suppose Putarr is configured to store downloads in the `/incoming` directory on Put.io.
You could use `rclone` to mount that directory locally with `rclone mount putio:/incoming /putarr`.
This would make the Put.io directory `/incoming` visible locally as `/putarr`. Finally, you would
configure Putarr with `download_dir: /putarr` to instruct its clients where to find downloaded
files.