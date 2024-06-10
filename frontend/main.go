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
	AlertsTopic        string `usage:"MQTT topic for incoming alerts" default:"alerts"`
	CORS               string `usage:"Value of Access-Control-Allow-Origin HTTP header - header will not be set if this is not set"`
	Docroot            string `usage:"HTML document root - will use the embedded docroot if not specified"`
	KeepAlive          string `usage:"The duration that Ollama should keep the model in memory" default:"300m"`
	MQTTBroker         string `usage:"MQTT broker URL" default:"tcp://localhost:1883" mandatory:"true"`
	OllamaModel        string `usage:"Model name used in query to Ollama" default:"llava"`
	OllamaURL          string `usage:"URL for the LLM REST endpoint" default:"http://localhost:11434/api/generate"`
	OpenAIModel        string `usage:"Model for the OpenAI API" default:"/mnt/models"`
	OpenAIPrompt       string `usage:"The prompt to be sent to the OpenAI model" default:"Does the text in the following paragraph describe a dangerous situation - answer yes or no"`
	OpenAIURL          string `usage:"URL for the OpenAI API" default:"http://localhost:8012/v1"`
	Port               int    `default:"8080" usage:"HTTP listener port"`
	Prompts            string `usage:"Path to file containing prompts to use - will use hardcoded prompts if this is not set"`
	SaveModelResponses bool   `usage:"Save model responses to a file"`
}

func main() {
	config := Config{}
	if err := configparser.Parse(&config); err != nil {
		log.Fatal(err)
	}

	shutdownCtx, cancelSignalNotify := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	var wg sync.WaitGroup

	http.HandleFunc("/healthz", healthHandler)

	sse := initializeSSEBroadcaster("/api/sse", config.CORS)
	http.HandleFunc("/api/ssestatus", internal.InitCORSMiddleware(config.CORS, sse.StatusHandler).Handler)
	sseCh := make(chan internal.SSEEvent, sseChannelSize)
	wg.Add(1)
	go func() {
		sse.Listen(sseCh)
		wg.Done()
	}()

	alertsController := internal.NewAlertsController(sseCh, config.OllamaURL, config.OllamaModel, config.KeepAlive, config.Prompts, config.OpenAIModel, config.OpenAIPrompt, config.OpenAIURL)
	if config.SaveModelResponses {
		alertsController.SaveModelResponses()
	}
	http.HandleFunc("/api/prompt", internal.InitCORSMiddleware(config.CORS, alertsController.PromptHandler).Handler)
	http.HandleFunc("/api/alertsstatus", internal.InitCORSMiddleware(config.CORS, alertsController.StatusHandler).Handler)
	http.HandleFunc("/api/resumeevents", internal.InitCORSMiddleware(config.CORS, alertsController.ResumeEventsHandler).Handler)
	wg.Add(1)
	go func() {
		alertsController.LLMChannelProcessor(shutdownCtx)
		close(sseCh)
		alertsController.Shutdown()
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
		if token := mqttClient.Subscribe(config.AlertsTopic, 1, controller.MQTTHandler); token.Wait() && token.Error() != nil {
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

func initializeSSEBroadcaster(uri, cors string) *internal.SSEBroadcaster {
	sse := internal.NewSSEBroadcaster()
	http.HandleFunc(uri, internal.InitCORSMiddleware(cors, sse.HTTPHandler).Handler)
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
