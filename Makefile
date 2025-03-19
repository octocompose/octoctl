.PHONY: build
build: 
	mkdir -p dist
	go build -tags 'netgo,disable_crypt' -buildmode=pie -trimpath -ldflags '-s' -o dist/octoctl -v ./cmd/octoctl

.PHONY: clean
clean:
	rm -rf dist

.PHONY: default
default: build