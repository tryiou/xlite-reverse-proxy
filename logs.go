package main

import (
	"compress/gzip"
	"io"
	"log"
	"os"
	"time"
)

var (
	logFile *os.File
	logger  *log.Logger
)

func initLogger() {
	var err error
	logFile, err = os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	logger = log.New(io.MultiWriter(os.Stdout, logFile), "", log.LstdFlags)

	// Start a goroutine to periodically check the log file size
	go func() {
		for {
			// Sleep for the specified interval before checking the file size again
			time.Sleep(5 * time.Minute)

			// Check if the log file exceeds the maximum size
			fileInfo, err := logFile.Stat()
			if err != nil {
				log.Println("Failed to get file information:", err)
				continue
			}

			if fileInfo.Size() >= int64(config.MaxLogSize) {
				// Rotate the log file
				err := rotateLogFile()
				if err != nil {
					log.Println("Failed to rotate log file:", err)
				}
			}
		}
	}()
}

func rotateLogFile() error {
	logFile.Close()

	// Rename the current log file with a timestamp suffix
	timestamp := time.Now().Format("20060102150405")
	backupFilePath := "logs_" + timestamp + ".txt"
	err := os.Rename("logs.txt", backupFilePath)
	if err != nil {
		return err
	}

	// Compress the rotated log file
	err = compressLogFile(backupFilePath)
	if err != nil {
		return err
	}
	// Remove the backup file
	err = os.Remove(backupFilePath)
	if err != nil {
		return err
	}
	// Create a new log file
	logFile, err = os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	logger.SetOutput(io.MultiWriter(os.Stdout, logFile))
	return nil
}

func compressLogFile(filePath string) error {
	source, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(filePath + ".gz")
	if err != nil {
		return err
	}
	defer destination.Close()

	gw := gzip.NewWriter(destination)
	defer gw.Close()

	_, err = io.Copy(gw, source)
	if err != nil {
		return err
	}

	return nil
}
