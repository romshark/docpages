# CI checks formatting with this gofumpt version.
GOFUMPT ?= go run mvdan.cc/gofumpt@latest

.PHONY: fmt gen

# Format module Go files with gofumpt.
fmt:
	$(GOFUMPT) -w .

# Refresh generate.go outputs, which CI requires current.
gen:
	go generate ./...
