BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: run

run:
	cd $(BASE) \
	&& \
	DOCROOT=$(BASE)/docroot go run .

test:
	cd $(BASE) \
	&& \
	go test -v ./...

remote:
	-if [ -f /tmp/port-forward-mqtt.pid ]; then \
	  kill `cat /tmp/port-forward-mqtt.pid`; \
	  rm -f /tmp/port-forward-mqtt.pid; \
	fi
	oc port-forward -n demo svc/mosquitto 1883:1883 & /bin/echo -n $$! > /tmp/port-forward-mqtt.pid
	oc port-forward -n demo svc/mistral-internal 8012:8012 & /bin/echo -n $$! > /tmp/port-forward-mistral.pid
	oc port-forward -n demo svc/llava-predictor-00001-private 8013:8012 & /bin/echo -n $$! > /tmp/port-forward-llava.pid
	sleep 3
	-DOCROOT=$(BASE)/docroot \
	 MQTTBROKER=tcp://localhost:1883 \
	 LLAVAURL=http://localhost:8013 \
	 OPENAIURL=http://localhost:8012/v1 \
	 OPENAIPROMPT="You are tailored to provide concise threat assessments. Reply with the level of threat, either low, medium or high. Explanations for assessments are not provided, maintaining a focus on clear, concise classification without additional commentary." \
	 PROMPTS=$(BASE)/../mocks/prompts.txt \
	 SAVEMODELRESPONSES=true \
	 go run .
	-kill `cat /tmp/port-forward-mqtt.pid`
	-rm -f /tmp/port-forward-mqtt.pid
	-kill `cat /tmp/port-forward-mistral.pid`
	-rm -f /tmp/port-forward-mistral.pid
	-kill `cat /tmp/port-forward-llava.pid`
	-rm -f /tmp/port-forward-llava.pid
