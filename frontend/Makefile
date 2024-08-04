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
	oc port-forward -n demo svc/llm-internal 8012:8012 & echo $$! > /tmp/port-forward-kserve.pid
	sleep 3
	-DOCROOT=$(BASE)/docroot \
	 LLAVAURL=http://`oc get -n demo route/llava -o jsonpath='{.spec.host}'` \
	 OPENAIPROMPT="You are tailored to provide concise threat assessments. Reply with the level of threat, either low, medium or high. Explanations for assessments are not provided, maintaining a focus on clear, concise classification without additional commentary." \
	 PROMPTS=$(BASE)/../mocks/prompts.txt \
	 SAVEMODELRESPONSES=false \
	 go run .
	-kill `cat /tmp/port-forward-mqtt.pid`
	-rm -f /tmp/port-forward-mqtt.pid
	-kill `cat /tmp/port-forward-kserve.pid`
	-rm -f /tmp/port-forward-kserve.pid
