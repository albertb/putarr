# Putarr

Putarr is a tool that lets you use Put.io to download your Radarr and Sonarr media.

## Features

- Put.io integration: Lets you use Put.io to torrent your media.
- Transmission API: Exposes a Transmission API for ease of integration with Radarr and Sonarr. Adding support for
  Lidarr and other *arrs would be trivial.
- Janitor: Cleans up successful Put.io transfers once the media has been imported.
- Downloader service: Downloads media locally from Put.io after a successful torrent transfer. Alternatively, mount your
  Put.io account with [rclone](http://rclone.org/) and let Radarr/Sonarr/etc. read from the drive.

## Installation

Build and run a docker image.

```sh
docker build -t putarr -f docker/Dockerfile .
```

```sh
docker run -d -v $HOME/.config/putarr:/config -v /media/downloads:/downloads -p 9091:9091 --name putarr putarr -v
```

## Configuration

Create a configuration file at `$HOME/.config/putarr/config.yaml` with the following structure:

```yaml
downloader:
  # When this field is set, download the media locally to this path once the Put.io transfer is finished.
  # Leave this unset to skip the download and instead rely on a rclone mount to access your media.
  dir: /path/to/download

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
    # account is used. You can find the ID of a directory in the URL when browsing your files
    # on the Put.io website.
    parent_dir_id: your_parent_dir_id

    # How often to clean up the transfers and files of successfully imported media.
    janitor_interval: 1h

    # The friend token used to identify the transfers belonging to this instance of Putarr
    # when multiple instances are running on the same Put.io account.
    friend_token: friend_token

radarr:
    url: http://localhost:7878
    api_key: your_radarr_api_key

sonarr:
    url: http://localhost:8989
    api_key: your_sonarr_api_key
```

## Download client

In Radarr and Sonarr, add a Transmission client with the username/password from the configuration file.
