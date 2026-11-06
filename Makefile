# gofumpt is the formatter that CI checks, at the same version.
GOFUMPT ?= go run mvdan.cc/gofumpt@latest

.PHONY: fmt gen

# fmt formats the Go files of the module with gofumpt.
fmt:
	$(GOFUMPT) -w .

# gen rewrites the committed outputs of generate.go, which CI requires current.
gen:
	go generate ./...
