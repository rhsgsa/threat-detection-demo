package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kwkoo/configparser"
)

func main() {
	config := struct {
		Port           int    `default:"8080" usage:"HTTP listener port"`
		Source         string `usage:"Source for responses" mandatory:"true"`
		LineSleepMsecs int    `default:"100" usage:"Delay between lines in milliseconds"`
		ResponseStop   string `default:"\"done\":true" usage:"Substring that denotes the end of a response"`
		ResponsePrefix string `usage:"Prefix for each response"`
	}{}
	if err := configparser.Parse(&config); err != nil {
		log.Fatal(err)
	}
	lines, err := readSource(config.Source)
	if err != nil {
		log.Fatalf("error reading from %s: %v", config.Source, err)
	}
	lineCount := len(lines)
	if lineCount < 1 {
		log.Fatal("could not read any lines from source")
	}
	currentLine := 0
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming is not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		for {
			line := lines[currentLine]
			if config.ResponsePrefix != "" {
				w.Write([]byte(config.ResponsePrefix))
			}
			w.Write(line)
			w.Write([]byte{'\n'})
			flusher.Flush()
			currentLine += 1
			if currentLine >= lineCount {
				currentLine = 0
				return
			}
			if strings.Contains(string(line), config.ResponseStop) {
				return
			}
			time.Sleep(time.Duration(config.LineSleepMsecs) * time.Millisecond)
		}
	})

	server := http.Server{
		Addr: fmt.Sprintf(":%d", config.Port),
	}
	var wg sync.WaitGroup
	shutdownCtx, cancelSignalNotify := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-shutdownCtx.Done()
		log.Print("shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("listening on port %v", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				log.Print("web server graceful shutdown")
				return
			}
			log.Fatal(err)
		}
	}()
	<-shutdownCtx.Done()
	cancelSignalNotify()
	log.Print("signal received, waiting for all goroutines to shut down...")
	wg.Wait()
	log.Print("all goroutines terminated")
}

func readSource(filename string) ([][]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines [][]byte
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		lines = append(lines, []byte(scanner.Text()))
	}
	return lines, nil
}
