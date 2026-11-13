# CI checks formatting with this gofumpt version.
GOFUMPT ?= go run mvdan.cc/gofumpt@latest

.PHONY: check fmt gen

# Format module Go files with gofumpt.
fmt:
	$(GOFUMPT) -w .

# Lint, format-check and type-check the TypeScript, as CI does.
check:
	pnpm -C web run check-all

# Refresh generate.go outputs, which CI requires current.
gen:
	go generate ./...
