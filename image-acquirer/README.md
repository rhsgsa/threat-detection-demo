# Image Acquirer

Grabs frames from a camera or video file and processes the frame with YOLO v8.

When specific classes are detected, it will send the image to a specific topic on an MQTT broker.

The `image-acquirer` can be configured with the following environment variables:

|Environment Variable|Default Value|Description|
|---|---|---|
|`PORT`|`8080`|Web server port number|
|`CAMERA`|`/dev/video0`|Filename of the camera device or video|
|`INTERESTED_CLASSES`||Comma-separated list of class indexes - when classes in this list are detected in a frame, a message will be sent to an MQTT topic|
|`MQTT_TOPIC`|`alerts`|MQTT topic to publish messages to|
|`MQTT_SERVER`|`localhost`|Hostname of the MQTT broker|
|`MQTT_PORT`|`1883`|Port number of the MQTT broker|
|`MQTT_TRANSPORT`|`tcp`|`websockets` is also valid|
|`FORCE_CPU`|`no`|Force YOLO to use the CPU for inferencing instead of the GPU - set to `yes` if you wish to activate this|
|`MODEL`|`best.pt`|YOLO model name|
|`CONFIDENCE`|`0.25`|Minimum confidence score for YOLO detection|


## Resources

*   [Paho Python Docs](https://eclipse.dev/paho/files/paho.mqtt.python/html/)
