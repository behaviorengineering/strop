.PHONY: help hooks-install

.DEFAULT_GOAL := help

help:
	@echo "strop — portable strop toolkit"
	@echo ""
	@echo "  make hooks-install  Install Lefthook git hooks (once per clone)"

hooks-install:
	@command -v lefthook >/dev/null 2>&1 || { \
		if command -v brew >/dev/null 2>&1; then brew install lefthook; \
		else go install github.com/evilmartians/lefthook@latest; fi; }
	@command -v lefthook >/dev/null 2>&1 || { echo "lefthook not on PATH; add $$(go env GOPATH)/bin"; exit 1; }
	lefthook install
