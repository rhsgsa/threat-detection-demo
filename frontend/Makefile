BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: run

run:
	cd $(BASE) \
	&& \
	DOCROOT=$(BASE)/docroot go run .
