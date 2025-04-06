# Putarr
Putarr is a tool that integrates with Put.io to download and manage your Radarr and Sonarr media.

## Features

- **Put.io Integration**: Uses Put.io to torrent your media seamlessly.
- **Transmission API**: Exposes a Transmission API for easy integration with Radarr and Sonarr. Adding support for Lidarr and other *arrs would be straightforward.
- **Janitor Service**: Automatically cleans up Put.io transfers after successful media import to avoid clutter.

## Installation

### Build the Docker Image
Run the following command to build the Docker image:

```sh
docker build -t putarr -f docker/Dockerfile .
```

## Run the Docker Container
Start the container with the following command:

```sh
docker run -d \
  -v $HOME/.config/putarr:/config \
  -v /media/downloads:/downloads \
  -p 9091:9091 \
  --name putarr \
  putarr -v
```

## Configuration

Create a configuration file at `$HOME/.config/putarr/config.yaml` with the following structure:

```yaml
transmission:
  # Credentials for clients (e.g., Radarr/Sonarr) to communicate with Putarr.
  username: your_username
  password: your_password

  # Path where downloads are available from the perspective of Radarr/Sonarr. This is the path where you've mounted your
  # Put.io account using rclone.
  download_dir: /path/to/download

putio:
  # OAuth token for Put.io communication.
  oauth_token: your_oauth_token

  # ID of the parent directory for downloaded files. Use -1 for the default directory. Find directory IDs in the URL
  # when browsing files on the Put.io website.
  parent_dir_id: your_parent_dir_id

  # Interval for cleaning up successfully imported transfers from Put.io.
  janitor_interval: 1h

  # Token to identify transfers for this Putarr instance when multiple instances use the same Put.io account.
  friend_token: foo

radarr:
  # Radarr API configuration.
  url: http://localhost:7878
  api_key: your_radarr_api_key

sonarr:
  # Sonarr API configuration.
  url: http://localhost:8989
  api_key: your_sonarr_api_key
```

## Download Client Setup
In Radarr and Sonarr, add a Transmission client with the username and password specified in the configuration file.

## Contributing
Contributions are welcome! Feel free to open issues or submit pull requests to improve Putarr.

## License
This project is licensed under the MIT License.