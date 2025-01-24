package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/albertb/putarr/internal"
	"github.com/albertb/putarr/internal/config"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	home, err := os.UserHomeDir()
	if err != nil {
		log.Println("failed to get home directory:", err)
	}
	defaultConfigPath := filepath.Join(home, ".config", "putarr", "putarr.yaml")

	addr := flag.String("addr", ":9091", "The network address to listen on")
	configPath := flag.String("config", defaultConfigPath, "Location of the config file")
	verbose := flag.Bool("v", false, "Whether to print verbose logs")
	dev := flag.Bool("dev", false, "Whether to run in development mode")

	flag.Parse()

	file, err := os.Open(*configPath)
	if err != nil {
		log.Fatalln("failed to open config file:", err)
	}
	defer file.Close()

	cfg, err := config.Read(file)
	if err != nil {
		log.Fatalln("failed to read config file:", err)
	}

	options := &config.Options{
		Config:      *cfg,
		Verbose:     *verbose,
		Development: *dev,
	}

	if err := internal.Run(*addr, options); err != nil {
		log.Fatalln("failed to run:", err)
	}
}
