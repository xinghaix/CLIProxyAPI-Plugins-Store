PLUGIN_ID := cpa-manager-plus
VERSION := 0.1.8

.PHONY: build tidy web clean

web:
	cd web && npm ci && npm run build

tidy:
	cd go && go mod tidy

build: web tidy
	cd go && CGO_ENABLED=1 go build -buildmode=c-shared -o ../$(PLUGIN_ID)-v$(VERSION).dylib .

clean:
	rm -rf go/web-dist web/node_modules
	rm -f $(PLUGIN_ID)-v$(VERSION).dylib $(PLUGIN_ID)-v$(VERSION).h $(PLUGIN_ID)_$(VERSION)_*.zip checksums.txt