BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: run

run:
	cd $(BASE) \
	&& \
	DOCROOT=$(BASE)/docroot go run .

remote:
	-if [ -f /tmp/port-forward.pid ]; then \
	  kill `cat /tmp/port-forward.pid`; \
	  rm -f /tmp/port-forward.pid; \
	fi
	oc port-forward -n demo svc/mosquitto 1883:1883 & echo $$! > /tmp/port-forward.pid
	sleep 3
	-DOCROOT=$(BASE)/docroot \
	 LLMURL=http://`oc get -n demo route/ollama -o jsonpath='{.spec.host}'`/api/generate \
	 go run .
	-kill `cat /tmp/port-forward.pid`
	-rm -f /tmp/port-forward.pid
