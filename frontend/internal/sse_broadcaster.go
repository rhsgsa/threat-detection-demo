package internal

import (
	"bytes"
	"log"
	"net/http"
	"sync"
	"time"
)

const pingIntervalSeconds = 30

type SSEEvent struct {
	EventType string
	Data      []byte
}

type SSEBroadcaster struct {
	clientMux    sync.RWMutex
	clients      map[chan []byte]struct{}
	wg           sync.WaitGroup
	shuttingDown bool // Set to true when shutting down, so we can't add any new clients
}

func NewSSEBroadcaster() *SSEBroadcaster {
	s := SSEBroadcaster{
		clients:      make(map[chan []byte]struct{}),
		shuttingDown: false,
	}
	return &s
}

// This should be run in a goroutine.
// Close the channel to terminate the goroutine.
func (b *SSEBroadcaster) Listen(in chan SSEEvent) {
	for event := range in {
		var buf bytes.Buffer
		buf.WriteString("event: ")
		buf.WriteString(event.EventType)
		buf.WriteString("\ndata: ")
		if event.Data != nil {
			buf.Write(event.Data)
		}
		buf.WriteString("\n\n")
		formattedMsg := buf.Bytes()
		b.clientMux.RLock()
		for clientCh := range b.clients {
			clientCh <- formattedMsg
		}
		b.clientMux.RUnlock()
	}

	log.Print("starting SSEBroadcaster.Listen() graceful shutdown...")
	b.clientMux.Lock()
	b.shuttingDown = true
	b.clientMux.Unlock()
	b.clientMux.RLock()
	for clientCh := range b.clients {
		close(clientCh)
	}
	b.clientMux.RUnlock()
	log.Print("waiting for all SSE clients to terminate...")
	b.wg.Wait()
	log.Print("SSEBroadcaster.Listen() graceful shutdown complete")
}

func (b *SSEBroadcaster) HTTPHandler(w http.ResponseWriter, r *http.Request) {
	b.wg.Add(1)
	defer b.wg.Done()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	log.Print("registering new SSE client...")
	ch := b.registerClient()
	if ch == nil {
		http.Error(w, "shutting down, unable to add new clients", http.StatusInternalServerError)
		return
	}
	pingTicker := time.NewTicker(pingIntervalSeconds * time.Second)
	defer func() {
		pingTicker.Stop()
		b.deregisterClient(ch)
		log.Print("SSE client connection shutdown")
	}()
	for {
		select {
		case <-r.Context().Done():
			log.Print("SSE client connection terminated")
			return
		case <-pingTicker.C:
			w.Write([]byte("event: ping\n\n"))
		case msg, ok := <-ch:
			if !ok {
				log.Print("SSE client channel closed")
				return
			}
			if _, err := w.Write(msg); err != nil {
				log.Printf("error writing to SSE client: %v", err)
				continue
			}
			flusher.Flush()
		}
	}
}

// The returned channel expects a formatted SSE event message
func (b *SSEBroadcaster) registerClient() chan []byte {
	if b.shuttingDown {
		return nil
	}
	b.clientMux.Lock()
	ch := make(chan []byte)
	b.clients[ch] = struct{}{}
	b.clientMux.Unlock()

	return ch
}

func (b *SSEBroadcaster) deregisterClient(ch chan []byte) {
	b.clientMux.Lock()
	delete(b.clients, ch)
	b.clientMux.Unlock()
}
