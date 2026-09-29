# proteng-conductor: drives the ML pipeline stage by stage and stores job state.
# Run `make` to list targets. Works from Git Bash, cmd or PowerShell (needs Git for Windows).

# On Windows, `bash` found from cmd or PowerShell is often the WSL launcher
# (C:\Windows\System32\bash.exe), so use the bash that ships with Git for Windows.
# Git's layout is <root>/mingw64/libexec/git-core and <root>/bin/bash.exe.
ifeq ($(OS),Windows_NT)
GIT_EXEC_PATH := $(shell git --exec-path)
ifneq ($(findstring /mingw64/libexec/git-core,$(GIT_EXEC_PATH)),)
SHELL := $(subst /mingw64/libexec/git-core,/bin/bash.exe,$(GIT_EXEC_PATH))
else
SHELL := bash
endif
else
SHELL := bash
endif
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS += --no-print-directory

# python3 on Windows is often the Microsoft Store stub, so check that it runs.
PYTHON ?= $(shell python3 -c 'import sys' >/dev/null 2>&1 && echo python3 || echo python)
IMAGE ?= proteng-conductor

.PHONY: help setup hooks env deps run mocks fmt lint test test-race check build docker-build

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "} /^##@/ {printf "\n%s\n", substr($$0, 5)} /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Setup
setup: hooks env deps ## First-time setup: git hooks, .env.local and Go modules (safe to re-run)

hooks: ## Install the git hooks (needs `pip install pre-commit`)
	$(PYTHON) -m pre_commit install --hook-type pre-commit --hook-type pre-push --hook-type commit-msg

env: ## Create .env.local from .env.example (never overwrites)
	@if [ -f .env.local ]; then echo ".env.local exists, left as is"; else cp .env.example .env.local; echo "created .env.local from .env.example"; fi

deps: ## Download the Go modules
	go mod download

##@ Run (needs RabbitMQ and MongoDB: make -C ../manual-guides-2023 infra-up)
run: ## Run with ENV=local (reads .env.local)
	bash run.sh

##@ Checks
fmt: ## Format every Go file with gofmt
	gofmt -l -w .

lint: ## Fail on files gofmt would change, then go vet
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "not gofmt'd (run: make fmt):"; echo "$$out"; exit 1; fi
	go vet ./...

test: ## Run the unit tests
	go test ./...

test-race: ## Run the unit tests with the race detector (needs cgo; on Windows a gcc on PATH)
	CGO_ENABLED=1 go test -race ./...

check: lint test ## Everything CI checks

mocks: ## Regenerate the gomock mocks from the go:generate comments
	@command -v mockgen >/dev/null 2>&1 || { echo "mockgen not found: see Mocks in README.md"; exit 1; }
	go generate ./...

build: ## Compile every package
	go build ./...

docker-build: ## Build the image locally
	docker build -t $(IMAGE) .
