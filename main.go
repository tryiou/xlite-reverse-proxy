package main

import (
	"flag"
	"log"
)

func init() {
	initLogger()
}

func main() {
	defer logFile.Close()

	dynlist := flag.Bool("dynlist", false, "Set to true to use dynamic server list & update routine")
	configFile := flag.String("config", "xlite-reverse-proxy-config.yaml", "Path to the configuration file")
	flag.Parse()

	// Create and initialize application
	app := NewApplication()

	if err := app.Initialize(*configFile); err != nil {
		log.Fatalf("Error initializing application: %v", err)
	}

	// Start the application
	if err := app.Start(*dynlist, DefaultPort); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}

	// Keep the main goroutine running
	select {}
}
