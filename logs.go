package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

var (
	logFile *os.File
	logger  *log.Logger
)

func initLogger() {
	var err error
	logFile, err = os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, FilePermissionRWXRWXR)
	if err != nil {
		log.Fatal(err)
	}

	logger = log.New(io.MultiWriter(os.Stdout, logFile), "", log.LstdFlags)

	// Start a goroutine to periodically check the log file size
	go func() {
		for {
			// Sleep for the specified interval before checking the file size again
			time.Sleep(LogRotationInterval)

			// Check if the log file exceeds the maximum size
			fileInfo, err := logFile.Stat()
			if err != nil {
				log.Println("Failed to get file information:", err)
				continue
			}

			cfg := globalConfig.GetConfig()
			if fileInfo.Size() >= int64(cfg.MaxLogSize) {
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
	logFile, err = os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, FilePermissionRWXRWXR)
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

// formatDuration renders a duration with two decimal places and a unit-appropriate
// suffix (s, ms, or µs) so timer values share a consistent, human-readable format.
func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.2fµs", float64(d.Nanoseconds())/float64(time.Microsecond))
	}
}

// logPrefixed logs a line whose namespace prefix is right-padded to
// LogPrefixColumnWidth so every message body starts at the same column.
func logPrefixed(prefix, format string, args ...any) {
	logger.Printf(fmt.Sprintf("%-*s", LogPrefixColumnWidth, prefix)+format, args...)
}

// coinNoServerState tracks the last "no server" event timestamp per coin so the
// relay path can emit a throttled one-shot "coin has no available server" event
// instead of one line per request during a backend outage.
var coinNoServerState = struct {
	sync.RWMutex
	lastLogged map[string]time.Time
}{lastLogged: make(map[string]time.Time)}

// logCoinNoServer logs a throttled "coin has no available server" event, at most
// once per coin within CoinOutageLogInterval. Returns true if a line was logged.
// The caller-supplied error carries the reason (coin absent, zero servers, etc.).
func logCoinNoServer(coin, tag string, err error) bool {
	now := time.Now()
	coinNoServerState.Lock()
	defer coinNoServerState.Unlock()
	if last, ok := coinNoServerState.lastLogged[coin]; ok && now.Sub(last) < CoinOutageLogInterval {
		return false
	}
	// Evict entries for coins that have stopped being requested (e.g. removed
	// from config); active outages refresh their own entry each window.
	for c, last := range coinNoServerState.lastLogged {
		if now.Sub(last) >= CoinOutageLogInterval {
			delete(coinNoServerState.lastLogged, c)
		}
	}
	coinNoServerState.lastLogged[coin] = now
	logPrefixed(LogPrefixRevProxy, " %s%s coin %s has no available server (%v)", tag, serverTag(-1), coin, err)
	return true
}

// logCoinRecovered logs a one-shot "service restored" event the first time a
// relay to the coin succeeds after a no-server outage.
func logCoinRecovered(coin string) {
	coinNoServerState.RLock()
	_, ok := coinNoServerState.lastLogged[coin]
	coinNoServerState.RUnlock()
	if !ok {
		return
	}
	coinNoServerState.Lock()
	defer coinNoServerState.Unlock()
	if _, ok := coinNoServerState.lastLogged[coin]; !ok {
		return
	}
	delete(coinNoServerState.lastLogged, coin)
	logPrefixed(LogPrefixRevProxy, " coin %s recovered, service restored", coin)
}

// canceledLogState aggregates client disconnects into a single line per window.
var canceledLogState = struct {
	sync.Mutex
	count      int
	lastLogged time.Time
}{}

// bumpCanceledCount records a context-canceled client disconnect and logs an
// aggregate count at most once per CanceledLogInterval.
func bumpCanceledCount() {
	canceledLogState.Lock()
	defer canceledLogState.Unlock()
	canceledLogState.count++
	if canceledLogState.lastLogged.IsZero() || time.Since(canceledLogState.lastLogged) >= CanceledLogInterval {
		logPrefixed(LogPrefixRevProxy, " %d client requests canceled mid-request", canceledLogState.count)
		canceledLogState.count = 0
		canceledLogState.lastLogged = time.Now()
	}
}

// startCanceledCountFlusher starts a background ticker that flushes the
// canceled-count accumulator on a fixed cadence, so a partially-filled window
// is still reported even if no new disconnects arrive to trigger a flush.
func startCanceledCountFlusher() {
	go func() {
		ticker := time.NewTicker(CanceledLogInterval)
		defer ticker.Stop()
		for range ticker.C {
			flushCanceledCount()
		}
	}()
}

// flushCanceledCount logs and resets the canceled-count accumulator if it holds
// any pending disconnects. When idle it leaves the state untouched so an idle
// tick never disturbs the next window.
func flushCanceledCount() {
	canceledLogState.Lock()
	defer canceledLogState.Unlock()
	if canceledLogState.count > 0 {
		logPrefixed(LogPrefixRevProxy, " %d client requests canceled mid-request", canceledLogState.count)
		canceledLogState.count = 0
		canceledLogState.lastLogged = time.Now()
	}
}
