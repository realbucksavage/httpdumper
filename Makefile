# Makefile for httpdumper
# Builds the server into the .builds directory

# Binary name
BIN := httpdumper
# Output directory
OUT := .builds
# Default GO env inherits from host; can be overridden: `make build GOOS=linux GOARCH=amd64`
GOOS ?=
GOARCH ?=
# Version metadata (best-effort)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Disable CGO for a static-ish binary by default
export CGO_ENABLED := 0

.PHONY: all build clean help

all: build

help:
	@echo "Targets:"
	@echo "  build       Build $(BIN) into $(OUT)/ for the current platform"
	@echo "  clean       Remove $(OUT) directory"
	@echo "Variables:"
	@echo "  GOOS, GOARCH (optional cross-compilation)"

# Ensure output directory exists
$(OUT):
	@mkdir -p $(OUT)

# Build current (or specified) platform binary
build: $(OUT)
	@echo "Building $(BIN) ($(if $(GOOS),GOOS=$(GOOS) ,)$(if $(GOARCH),GOARCH=$(GOARCH),)) -> $(OUT)/$(BIN)"
	$(if $(GOOS),GOOS=$(GOOS) ,) $(if $(GOARCH),GOARCH=$(GOARCH) ,) go build -ldflags="$(LDFLAGS)" -o $(OUT)/$(BIN) .
	@echo "Built $(OUT)/$(BIN)"

clean:
	@rm -rf $(OUT)
	@echo "Cleaned $(OUT)"
