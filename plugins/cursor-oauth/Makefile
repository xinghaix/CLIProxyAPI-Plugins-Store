PLUGIN_ID := cursor-oauth
VERSION := 0.6.3

.PHONY: build test tidy package-all clean

tidy:
	cd go && go mod tidy

test:
	cd go && go test ./...

build: tidy
	cd go && CGO_ENABLED=1 go build -buildmode=c-shared -ldflags="-X main.pluginVersion=$(VERSION)" -o ../$(PLUGIN_ID)-v$(VERSION).dylib .

package-all:
	./packaging/build-store-release.sh $(VERSION)

clean:
	rm -f $(PLUGIN_ID)-v*.dylib $(PLUGIN_ID)-v*.so $(PLUGIN_ID)-v*.dll $(PLUGIN_ID)-v*.h
	rm -rf release
