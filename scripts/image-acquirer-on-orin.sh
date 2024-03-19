#!/bin/bash

IMAGE=ghcr.io/kwkoo/image-acquirer
MQTT_SERVER=mosquitto-demo.apps.replace.me

podman run \
  --name image-acquirer \
  --rm \
  -it \
  -p 8080:8080 \
  --runtime /usr/bin/nvidia-container-runtime \
  --group-add keep-groups \
  --security-opt label=disable \
  --env NVIDIA_DRIVER_CAPABILITIES=all \
  --env NVIDIA_VISIBLE_DEVICES=0 \
  --env CAMERA=/videos/video.mp4 \
  --env MQTT_SERVER=$MQTT_SERVER \
  --env MQTT_PORT=80 \
  --env MQTT_TRANSPORT=websockets \
  --env MQTT_TOPIC=alerts \
  --env MODEL=NCS_YOLOv8-20Epochs.pt \
  --env CONFIDENCE=0.5 \
  -v ./Gun_Video.mp4:/videos/video.mp4 \
  $IMAGE
