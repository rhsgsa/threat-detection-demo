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
	oc port-forward -n demo svc/mosquitto 1883:1883 & echo $$! > /tmp/port-forward-mqtt.pid
	sleep 3
	-DOCROOT=$(BASE)/docroot \
	 OLLAMAMODEL="llava:34b-v1.6" \
	 OLLAMAURL=http://`oc get -n demo route/ollama -o jsonpath='{.spec.host}'`/api/generate \
	 go run .
	-kill `cat /tmp/port-forward-mqtt.pid`
	-rm -f /tmp/port-forward-mqtt.pid
