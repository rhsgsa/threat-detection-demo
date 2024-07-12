# Image Acquirer

Grabs frames from a camera or video file and processes the frame with YOLO v8.

When specific classes are detected, it will send the image to a specific topic on an MQTT broker.

The `image-acquirer` can be configured with the following environment variables:

|Environment Variable|Default Value|Description|
|---|---|---|
|`CAMERA`|`/dev/video0`|Filename of the camera device or video|
|`CONFIDENCE`|`0.25`|Minimum confidence score for YOLO detection|
|`FORCE_CPU`|`no`|Force YOLO to use the CPU for inferencing instead of the GPU - set to `yes` if you wish to activate this|
|`INTERESTED_CLASSES`||Comma-separated list of class indexes - when classes in this list are detected in a frame, a message will be sent to an MQTT topic; if this is not set then messages will be sent whenever any class is detected|
|`MODEL`|`best.pt`|YOLO model name|
|`MQTT_PORT`|`1883`|Port number of the MQTT broker|
|`MQTT_PUBLISH_TIMEOUT`|`3`|Number of seconds to wait for a message to be published successfully|
|`MQTT_SERVER`|`localhost`|Hostname of the MQTT broker|
|`MQTT_TOPIC`|`alerts`|MQTT topic to publish messages to|
|`MQTT_TRANSPORT`|`tcp`|`websockets` is also valid|
|`PORT`|`8080`|Web server port number|
|`RESIZE`||Resolution to resize the image to, in the form `widthxheight` (e.g. `640x480`); will not resize if not set|
|`TRACKING`|`yes`|If set to `yes`, YOLO's tracker will be used, and an event will be sent to the MQTT broker when a new tracker ID is found.<br/><br/>If set to `no`, a new event will be sent to the MQTT broker every time an object is detected in the frame|


## Resources

*   [Paho Python Docs](https://eclipse.dev/paho/files/paho.mqtt.python/html/)
*   [Exporting a YOLO model in the TensorRT format](https://docs.ultralytics.com/integrations/tensorrt/#usage)
