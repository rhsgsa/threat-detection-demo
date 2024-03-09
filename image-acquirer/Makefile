BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: run

run:
	cd $(BASE) \
	&& \
	PORT=8000 \
	MQTT_SERVER=localhost \
	MQTT_TOPIC=alerts \
	INTERESTED_CLASSES=1 \
	CAMERA=../../redhat-pins-ai-demo/container/video.mp4 \
	python3 ./app/app.py
