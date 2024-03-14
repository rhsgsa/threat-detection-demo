BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
CONDA_ROOT=${HOME}/Downloads/miniconda3
CONDA_ENV=yolo

.PHONY: run

run:
	cd $(BASE) \
	&& \
	. $(CONDA_ROOT)/etc/profile.d/conda.sh \
	&& \
	conda activate $(CONDA_ENV) \
	&& \
	PORT=8000 \
	MQTT_SERVER=localhost \
	MQTT_TOPIC=alerts \
	INTERESTED_CLASSES=1 \
	CAMERA=../../redhat-pins-ai-demo/container/video.mp4 \
	python3 ./app/app.py

t:
	cd $(BASE) \
	&& \
	. $(CONDA_ROOT)/etc/profile.d/conda.sh \
	&& \
	conda activate $(CONDA_ENV) \
	&& \
	PORT=8000 \
	MQTT_SERVER=localhost \
	MQTT_TOPIC=alerts \
	CONFIDENCE=0.5 \
	CAMERA=/Users/kkoo/Downloads/Gun_Video.mp4 \
	MODEL=NCS_YOLOv8-20Epochs.pt \
	python3 ./app/app.py
