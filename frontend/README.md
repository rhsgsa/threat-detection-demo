# Threat Detection Frontend

## Configuration

|Environment Variable|Default Value|Description|
|---|---|---|
|`ALERTSTOPIC`|`alerts`|MQTT topic for incoming alerts|
`CORS`||Value of `Access-Control-Allow-Origin` HTTP header - header will not be set if this is not set|
|`DOCROOT`||HTML document root - will use the embedded docroot if not specified|
|`KEEPALIVE`|`300m`|The duration that Ollama should keep the model in memory|
|`LLMMODEL`|`llava`|Model name used in query to Ollama|
|`LLMURL`|`http://localhost:11434/api/generate`|URL for the LLM REST endpoint|
|`MQTTBROKER`|`tcp://localhost:1883`|MQTT broker URL|
|`PORT`|`8080`|Web server port|
|`PROMPTS`||Path to file containing prompts to use - will use hardcoded prompts if this is not set|


## Prompts File

*   You can configure the frontend to use custom prompts by placing your prompts in a text file and setting the `PROMPTS` environment variable

*   Put each prompt on a separate line

		What is your name?
		How old are you?
		Why is this happening?

*   Each prompt can also have a short version and a descriptive version; the short version is used in the web UI and the descriptive version is sent to the LLM; use a pipe character (`|`) to delimit the short version and the descriptive version

		What|What is your name?
		How|How old are you?
		Why|Why is this happening?


## Testing with mocks

*   Start up mock `image-acquirer`, `frontend`, mock `ollama`, then bring `frontend` container down

		docker compose -f ../yaml/docker-compose/frontend-with-mocks.yaml up

		docker stop frontend

*   Run `frontend` locally

		make run


## Sound File

The sound file was downloaded from [pixabay.com](https://pixabay.com/service/terms/)
