#!/bin/bash

# This script runs the image-acquirer using a webcam as a source
# Ensure that /dev/video0 exists before running this script
# Don't forget to set MQTT_SERVER to point to your actual server

IMAGE=quay.io/rhsgsa/image-acquirer
MQTT_SERVER=mosquitto-demo.apps.replace.me

podman run \
  --name image-acquirer \
  --rm \
  -it \
  -p 8080:8080 \
  --device /dev/video0:/dev/video0 \
  --runtime /usr/bin/nvidia-container-runtime \
  --group-add keep-groups \
  --security-opt label=disable \
  --env NVIDIA_DRIVER_CAPABILITIES=all \
  --env NVIDIA_VISIBLE_DEVICES=0 \
  --env CAMERA=/dev/video0 \
  --env MQTT_SERVER=$MQTT_SERVER \
  --env MQTT_PORT=80 \
  --env MQTT_TRANSPORT=websockets \
  --env MQTT_TOPIC=alerts \
  --env CONFIDENCE=0.5 \
  $IMAGE
