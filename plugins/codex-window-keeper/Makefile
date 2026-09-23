PLUGIN_ID := codex-window-keeper
VERSION := 0.1.0
GOOS ?= $(shell go env GOOS)

ifeq ($(GOOS),windows)
EXT := dll
else ifeq ($(GOOS),darwin)
EXT := dylib
else
EXT := so
endif

.PHONY: build tidy test clean

tidy:
	cd go && go mod tidy

test:
	cd go && go test ./...

build: tidy test
	cd go && CGO_ENABLED=1 go build -buildmode=c-shared -ldflags "-X main.pluginVersion=$(VERSION)" -o ../$(PLUGIN_ID)-v$(VERSION).$(EXT) .

clean:
	rm -f $(PLUGIN_ID)-v$(VERSION).dylib $(PLUGIN_ID)-v$(VERSION).so $(PLUGIN_ID)-v$(VERSION).dll $(PLUGIN_ID)-v$(VERSION).h
