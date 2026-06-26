PLUGIN_ID := cpa-manager-plus
VERSION := 0.1.0

.PHONY: build tidy
tidy:
	cd go && go mod tidy
build: tidy
	cd go && CGO_ENABLED=1 go build -buildmode=c-shared -o ../$(PLUGIN_ID)-v$(VERSION).dylib .
