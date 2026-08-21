package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cookiengineer/zimdex/internal/server"
	"github.com/cookiengineer/zimdex/io/zimfs"
)

func resolvePath(raw string) string {
	raw = strings.TrimSpace(raw)

	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			raw = home + "/" + raw[2:]
		}
	}

	return raw
}

func main() {
	folder := "./data"
	port := 3000

	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "--folder=") {
			val := arg[9:]
			val = strings.Trim(val, "\"")
			val = resolvePath(val)

			stat, err := os.Stat(val)
			if err == nil && stat.IsDir() {
				folder = val
			} else {
				log.Printf("Warning: --folder=%s is not a valid directory, using %s", val, folder)
			}
		} else if strings.HasPrefix(arg, "--port=") {
			val := arg[7:]
			n, err := strconv.Atoi(val)
			if err == nil && n > 0 && n < 65535 {
				port = n
			} else {
				log.Printf("Warning: --port=%s is not valid, using %d", val, port)
			}
		}
	}

	if err := os.MkdirAll(folder, 0755); err != nil {
		log.Fatalf("Cannot create data directory %s: %v", folder, err)
	}

	log.Printf("ZIMdex")
	log.Printf("- Folder: %s", folder)
	log.Printf("- Port:   %d", port)

	manager := zimfs.NewManager(folder)

	if err := manager.Scan(); err != nil {
		log.Printf("Warning: Could not scan for ZIM files: %v", err)
	} else {
		archives := manager.List()
		log.Printf("Loaded %d ZIM archive(s)", len(archives))
	}

	srv := server.NewServer(folder, port)

	go func() {
		if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("Server error: %v", err)
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	<-signalCh

	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	if err := manager.Close(); err != nil {
		log.Printf("Manager close error: %v", err)
	}

	log.Println("Goodbye.")
}
