package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/albertb/putarr/internal"
)

// A dumb little client to debug the server. It calls torrent-get and prints the response.
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("failed to get user home directory: ", err)
	}
	defaultConfigPath := filepath.Join(home, ".config", "putarr", "config.yaml")

	server := flag.String("server", "localhost:9099", "`address` to connect to")
	configPath := flag.String("config", defaultConfigPath, "configuration file")

	flag.Parse()

	file, err := os.Open(*configPath)
	if err != nil {
		log.Fatalln("failed to open config file:", err)
	}
	defer file.Close()

	config, err := internal.ReadConfig(file)
	if err != nil {
		log.Fatalln("failed to read config file:", err)
	}

	// 1. Get the session token from the server.
	req, err := http.NewRequest("GET", "http://"+*server, nil)
	if err != nil {
		log.Fatalln("failed to create request:", err)
	}
	req.SetBasicAuth(config.Transmission.Username, config.Transmission.Password)

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalln("failed to perform request:", err)
	}
	token := resp.Header.Get("X-Transmission-Session-Id")

	// 2. Using the session token, get a list of all the torrents.
	payload := `{
		"method": "torrent-get",
		"arguments": {}
	}`

	req, err = http.NewRequest("POST", "http://"+*server+"/transmission/rpc", bytes.NewBufferString(payload))
	if err != nil {
		log.Fatalln("failed to create request:", err)
	}

	req.SetBasicAuth(config.Transmission.Username, config.Transmission.Password)
	req.Header.Set("X-Transmission-Session-Id", token)

	resp, err = client.Do(req)
	if err != nil {
		log.Fatalln("failed to perform request:", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("failed to read response body:", err)
	}

	// 3. Pretty-print the response.
	var response bytes.Buffer
	err = json.Indent(&response, body, "", "  ")
	if err != nil {
		log.Fatalln("failed to indent JSON:", err)
	}

	log.Println("response:\n", response.String())
}
