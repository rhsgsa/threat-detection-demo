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
	CAMERA=../../Gun_Video.mp4 \
	TRACKING=yes \
	python3 ./app/app.py

t:
	cd $(BASE) \
	&& \
	. $(CONDA_ROOT)/etc/profile.d/conda.sh \
	&& \
	conda activate $(CONDA_ENV) \
	&& \
	PORT=8000 \
	MQTT_SERVER="" \
	MQTT_TOPIC=alerts \
	CONFIDENCE=0.5 \
	CAMERA=/Users/kkoo/Downloads/yolo-training/test/videos/combined.mp4 \
	MODEL=/Users/kkoo/Downloads/yolo-training/test/20240628-small.pt \
	python3 ./app/app.py
