.PHONY: help licenses licenses-check licenses-install

GO_LICENSES := $(shell which go-licenses 2>/dev/null)

help:
	@echo "Available targets:"
	@echo "  licenses         - Check licenses of all dependencies (installs go-licenses if needed)"
	@echo "  licenses-check   - Check licenses without installing"
	@echo "  licenses-install - Install go-licenses tool"

licenses-install:
	@echo "Installing go-licenses..."
	go install github.com/google/go-licenses@latest

licenses:
ifndef GO_LICENSES
	@echo "go-licenses not found, installing..."
	@$(MAKE) licenses-install
endif
	@echo "Checking licenses..."
	go-licenses csv ./... 2>/dev/null | column -t -s,

licenses-check:
ifdef GO_LICENSES
	go-licenses csv ./... 2>/dev/null | column -t -s,
else
	@echo "Error: go-licenses not installed. Run 'make licenses-install' first."
	@exit 1
endif
