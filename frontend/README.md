# Threat Detection Frontend

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


## Sound File

The sound file was downloaded from [pixabay.com](https://pixabay.com/service/terms/)
