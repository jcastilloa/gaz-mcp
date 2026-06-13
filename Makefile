.PHONY: help build install release checksums publish clean clean-dist ensure-tools

GO ?= go
SERVICE_NAME ?= gaz-mcp
CMD_DIR := ./cmd/server
BIN_DIR := ./bin
BIN := $(BIN_DIR)/$(SERVICE_NAME)
DIST_DIR := ./dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_PACKAGE := github.com/jcastillo/gaz-mcp/shared/buildinfo
LDFLAGS_VERSION := -X $(VERSION_PACKAGE).Version=$(VERSION)
BUILD_LDFLAGS ?= $(LDFLAGS_VERSION)
RELEASE_LDFLAGS ?= -s -w $(LDFLAGS_VERSION)
PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
RELEASE_BASENAME := $(SERVICE_NAME)_$(VERSION)
RELEASE_FILES := README.md config.sample.yaml

help:
	@echo "Targets:"
	@echo "  make build        - Compile binary to $(BIN)"
	@echo "  make install      - Install binary in $$HOME/.local/bin"
	@echo "  make release      - Build release artifacts in $(DIST_DIR) for $(PLATFORMS)"
	@echo "  make checksums    - Generate SHA256 checksums for current release artifacts"
	@echo "  make publish      - Create GitHub release with artifacts (requires gh auth)"
	@echo "  make clean        - Remove local build artifacts"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION=<value>   - Override release version (default: git describe)"
	@echo "  PLATFORMS=<list>  - Override target matrix (default: $(PLATFORMS))"

build:
	@mkdir -p $(BIN_DIR)
	@$(GO) build -ldflags "$(BUILD_LDFLAGS)" -o $(BIN) $(CMD_DIR)
	@echo "built: $(BIN)"

install: build
	@mkdir -p $(HOME)/.local/bin
	@install -m 0755 $(BIN) $(HOME)/.local/bin/$(SERVICE_NAME)
	@echo "installed: $(HOME)/.local/bin/$(SERVICE_NAME)"

ensure-tools:
	@command -v zip >/dev/null
	@command -v sha256sum >/dev/null

release: ensure-tools clean-dist
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		stage="$(RELEASE_BASENAME)_$${os}_$${arch}"; \
		stage_dir="$(DIST_DIR)/$$stage"; \
		bin_name="$(SERVICE_NAME)"; \
		if [ "$$os" = "windows" ]; then bin_name="$(SERVICE_NAME).exe"; fi; \
		echo "building $$os/$$arch"; \
		mkdir -p "$$stage_dir"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o "$$stage_dir/$$bin_name" $(CMD_DIR); \
		for file in $(RELEASE_FILES); do \
			if [ -f "$$file" ]; then cp "$$file" "$$stage_dir/"; fi; \
		done; \
		if [ "$$os" = "windows" ]; then \
			(cd "$(DIST_DIR)" && zip -qr "$$stage.zip" "$$stage"); \
		else \
			tar -C "$(DIST_DIR)" -czf "$(DIST_DIR)/$$stage.tar.gz" "$$stage"; \
		fi; \
		rm -rf "$$stage_dir"; \
	done
	@$(MAKE) checksums VERSION="$(VERSION)"
	@echo "release artifacts ready in $(DIST_DIR)"

checksums:
	@files=$$(find "$(DIST_DIR)" -maxdepth 1 -type f \( -name "$(RELEASE_BASENAME)_*.tar.gz" -o -name "$(RELEASE_BASENAME)_*.zip" \) | sed 's|.*/||' | sort); \
	if [ -z "$$files" ]; then \
		echo "no release artifacts found for $(RELEASE_BASENAME) in $(DIST_DIR)"; \
		exit 1; \
	fi; \
	(cd "$(DIST_DIR)" && sha256sum $$files > "$(RELEASE_BASENAME)_checksums.txt"); \
	echo "generated: $(DIST_DIR)/$(RELEASE_BASENAME)_checksums.txt"

publish: release
	@gh release create "$(VERSION)" \
		"$(DIST_DIR)/$(RELEASE_BASENAME)"*.tar.gz \
		"$(DIST_DIR)/$(RELEASE_BASENAME)"*.zip \
		"$(DIST_DIR)/$(RELEASE_BASENAME)_checksums.txt" \
		--title "$(VERSION)" \
		--notes "Release $(VERSION)"

clean:
	@rm -rf $(BIN_DIR)
	@echo "cleaned: $(BIN_DIR)"

clean-dist:
	@rm -rf $(DIST_DIR)
	@echo "cleaned: $(DIST_DIR)"
