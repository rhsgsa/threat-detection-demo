#!/bin/bash

# This script runs the image-acquirer using a video file as a source
# It runs standalone without an MQTT_SERVER

IMAGE=ghcr.io/kwkoo/image-acquirer

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
  --env MQTT_SERVER="" \
  --env MQTT_PORT=80 \
  --env MQTT_TRANSPORT=websockets \
  --env MQTT_TOPIC="" \
  --env CONFIDENCE=0.5 \
  -v ./Gun_Video.mp4:/videos/video.mp4 \
  $IMAGE
