package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/kwkoo/configparser"
	"github.com/kwkoo/threat-detection-frontend/internal"
)

const sseChannelSize = 50

//go:embed docroot/*
var content embed.FS

type Config struct {
	Port        int    `default:"8080" usage:"HTTP listener port"`
	Docroot     string `usage:"HTML document root - will use the embedded docroot if not specified"`
	MQTTBroker  string `usage:"MQTT broker URL" default:"tcp://localhost:1883" mandatory:"true"`
	AlertsTopic string `usage:"MQTT topic for incoming alerts" default:"alerts"`
	LLMURL      string `usage:"URL for the LLM REST endpoint" default:"http://localhost:11434/api/generate"`
	Prompts     string `usage:"Path to file containing prompts to use - will use hardcoded prompts if this is not set"`
}

func main() {
	config := Config{}
	if err := configparser.Parse(&config); err != nil {
		log.Fatal(err)
	}

	shutdownCtx, cancelSignalNotify := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	var wg sync.WaitGroup

	http.HandleFunc("/healthz", healthHandler)

	sse := initializeSSEBroadcaster("/api/sse")
	http.HandleFunc("/api/ssestatus", sse.StatusHandler)
	sseCh := make(chan internal.SSEEvent, sseChannelSize)
	wg.Add(1)
	go func() {
		sse.Listen(sseCh)
		wg.Done()
	}()

	alertsController := internal.NewAlertsController(sseCh, config.LLMURL, config.Prompts)
	http.HandleFunc("/api/prompt", alertsController.PromptHandler)
	http.HandleFunc("/api/alertsstatus", alertsController.StatusHandler)
	wg.Add(1)
	go func() {
		alertsController.LLMChannelProcessor(shutdownCtx)
		close(sseCh)
		wg.Done()
	}()

	mqttClient := initializeMQTTClient(config, alertsController)
	wg.Add(1)
	go func() {
		<-shutdownCtx.Done()
		shutdownMQTTClient(mqttClient)
		wg.Done()
	}()

	filesystem := initializeDocroot(config.Docroot)
	http.HandleFunc("/", http.FileServer(filesystem).ServeHTTP)

	server := initWebServer(shutdownCtx, config.Port)
	wg.Add(1)
	go func() {
		startWebServer(server)
		wg.Done()
	}()

	<-shutdownCtx.Done()
	cancelSignalNotify()
	log.Print("signal received, waiting for all goroutines to shut down...")
	wg.Wait()
	log.Print("all goroutines terminated")
}

func initializeMQTTClient(config Config, controller *internal.AlertsController) MQTT.Client {
	opts := MQTT.NewClientOptions()
	opts.AddBroker(config.MQTTBroker)
	opts.SetAutoReconnect(true)
	opts.OnConnect = func(mqttClient MQTT.Client) {
		if token := mqttClient.Subscribe(config.AlertsTopic, 1, controller.AlertsHandler); token.Wait() && token.Error() != nil {
			log.Fatalf("could not subscribe to %s: %v", config.AlertsTopic, token.Error())
		}
	}

	mqttClient := MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("error connecting to MQTT broker %s: %v", config.MQTTBroker, token.Error())
	}
	log.Printf("successfully connected to MQTT broker %s", config.MQTTBroker)

	return mqttClient
}

func shutdownMQTTClient(mqttClient MQTT.Client) {
	log.Print("shutting down MQTT client...")
	mqttClient.Disconnect(5000)
	log.Print("MQTT client successfully shutdown")
}

func initializeDocroot(path string) http.FileSystem {
	if len(path) > 0 {
		log.Printf("using %s in the file system as the document root", path)
		return http.Dir(path)
	} else {
		log.Print("using the embedded filesystem as the docroot")

		subdir, err := fs.Sub(content, "docroot")
		if err != nil {
			log.Fatalf("could not get subdirectory: %v", err)
		}
		return http.FS(subdir)
	}
}

func initializeSSEBroadcaster(uri string) *internal.SSEBroadcaster {
	sse := internal.NewSSEBroadcaster()
	http.HandleFunc(uri, sse.HTTPHandler)
	return sse
}

func initWebServer(shutdownCtx context.Context, port int) *http.Server {
	server := http.Server{
		Addr:        fmt.Sprintf(":%d", port),
		ReadTimeout: 5 * time.Second,
	}

	go func(shutdownCtx context.Context) {
		<-shutdownCtx.Done()
		log.Print("shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}(shutdownCtx)

	return &server
}

func startWebServer(server *http.Server) {
	log.Printf("listening on port %v", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			log.Print("web server graceful shutdown")
			return
		}
		log.Fatal(err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "OK")
}
