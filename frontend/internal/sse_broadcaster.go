package internal

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

const pingIntervalSeconds = 15
const clientChannelSize = 50

type SSEEvent struct {
	EventType string
	Data      []byte
}

type SSEBroadcaster struct {
	clientMux    sync.RWMutex
	clients      map[chan []byte]string
	wg           sync.WaitGroup
	shuttingDown bool // Set to true when shutting down, so we can't add any new clients
}

func NewSSEBroadcaster() *SSEBroadcaster {
	s := SSEBroadcaster{
		clients:      make(map[chan []byte]string),
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
		if event.Data != nil && len(event.Data) > 0 {
			buf.Write(event.Data)
		}
		buf.WriteString("\n\n")
		formattedMsg := buf.Bytes()
		b.clientMux.RLock()
		for clientCh := range b.clients {
			select {
			case clientCh <- formattedMsg:
				// sent successfully
				continue
			default:
				log.Printf("SSE client channel %s full", b.clients[clientCh])
			}
		}
		b.clientMux.RUnlock()
	}

	log.Print("starting SSEBroadcaster.Listen() graceful shutdown...")
	b.clientMux.Lock()
	b.shuttingDown = true
	for clientCh := range b.clients {
		close(clientCh)
		delete(b.clients, clientCh)
	}
	b.clientMux.Unlock()
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

	// Used to set write timeouts
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	log.Print("registering new SSE client...")
	ch := b.registerClient(r.RemoteAddr)
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
	log.Print("SSE HTTP handler loop")
	for {
		select {
		case <-r.Context().Done():
			log.Printf("SSE client connection %s terminated", r.RemoteAddr)
			return
		case <-pingTicker.C:
			if err := writeWithTimeout(rc, w, []byte("event: ping\n\n")); err != nil {
				log.Printf("error writing to SSE client %s: %v", r.RemoteAddr, err)
				return
			}
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				log.Printf("SSE client channel %s closed", r.RemoteAddr)
				return
			}
			if err := writeWithTimeout(rc, w, msg); err != nil {
				log.Printf("error writing to SSE client %s: %v", r.RemoteAddr, err)
				return
			}
			flusher.Flush()
		}
	}
}

func writeWithTimeout(rc *http.ResponseController, w http.ResponseWriter, data []byte) error {
	err := rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (b *SSEBroadcaster) StatusHandler(w http.ResponseWriter, r *http.Request) {
	clientChannels := make(map[string]int)
	b.clientMux.RLock()
	for ch, address := range b.clients {
		clientChannels[address] = len(ch)
	}
	b.clientMux.RUnlock()
	status := struct {
		ClientChannels map[string]int `json:"client_channels"`
	}{
		ClientChannels: clientChannels,
	}
	json.NewEncoder(w).Encode(&status)
}

// The returned channel expects a formatted SSE event message
func (b *SSEBroadcaster) registerClient(clientAddress string) chan []byte {
	if b.shuttingDown {
		return nil
	}
	b.clientMux.Lock()
	ch := make(chan []byte, clientChannelSize)
	b.clients[ch] = clientAddress
	b.clientMux.Unlock()

	return ch
}

func (b *SSEBroadcaster) deregisterClient(ch chan []byte) {
	b.clientMux.Lock()
	delete(b.clients, ch)
	b.clientMux.Unlock()
}
