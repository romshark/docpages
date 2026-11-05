# gofumpt is the formatter that CI checks, at the same version.
GOFUMPT ?= go run mvdan.cc/gofumpt@latest

.PHONY: fmt

# fmt formats the Go files of the module with gofumpt.
fmt:
	$(GOFUMPT) -w .
