SHELL := /bin/zsh

PROJECT_ROOT := $(realpath $(dir $(lastword $(MAKEFILE_LIST))))
BACKEND_DIR := $(PROJECT_ROOT)/backend
BACKEND_PYTHON_DIR := $(PROJECT_ROOT)/backend-python
FRONTEND_DIR := $(PROJECT_ROOT)/frontend
FRONTEND_ENV_FILE := $(FRONTEND_DIR)/.env.local
GO_CACHE_DIR := $(PROJECT_ROOT)/.cache/go-build
GO_MOD_CACHE_DIR := $(PROJECT_ROOT)/.cache/go-mod
BACKEND_PYTHON_VENV := $(BACKEND_PYTHON_DIR)/.venv
BACKEND_PYTHON_BIN := $(BACKEND_PYTHON_VENV)/bin/python
BACKEND_PYTHON_PIP := $(BACKEND_PYTHON_VENV)/bin/pip

ifneq (,$(filter run,$(MAKECMDGOALS)))
RUN_TARGET := $(word 2,$(MAKECMDGOALS))
endif

.PHONY: help run backend backend-python frontend run-backend run-backend-python run-frontend setup-backend setup-backend-python setup-frontend test

help:
	@echo "Usage:"
	@echo "  make run backend"
	@echo "  make run backend-python"
	@echo "  make run frontend"
	@echo "  make setup-backend"
	@echo "  make setup-backend-python"
	@echo "  make setup-frontend"
	@echo "  make test"

test:
	@cd "$(BACKEND_DIR)" && GOCACHE="$(GO_CACHE_DIR)" GOMODCACHE="$(GO_MOD_CACHE_DIR)" go test ./internal/...

run:
ifeq ($(RUN_TARGET),backend)
	@$(MAKE) run-backend
else ifeq ($(RUN_TARGET),frontend)
	@$(MAKE) run-frontend
else ifeq ($(RUN_TARGET),backend-python)
	@$(MAKE) run-backend-python
else
	@echo "Usage: make run backend | make run backend-python | make run frontend"
	@exit 1
endif

setup-backend:
	@mkdir -p "$(GO_CACHE_DIR)" "$(GO_MOD_CACHE_DIR)"
	@cd "$(BACKEND_DIR)" && GOCACHE="$(GO_CACHE_DIR)" GOMODCACHE="$(GO_MOD_CACHE_DIR)" go mod tidy

backend:
	@:

run-backend: setup-backend
	@mkdir -p "$(GO_CACHE_DIR)" "$(GO_MOD_CACHE_DIR)"
	@cd "$(BACKEND_DIR)" && if [ -f .env ]; then set -a; source .env; set +a; fi && GOCACHE="$(GO_CACHE_DIR)" GOMODCACHE="$(GO_MOD_CACHE_DIR)" go run ./cmd/api

setup-backend-python:
	@if [ ! -d "$(BACKEND_PYTHON_VENV)" ]; then python3 -m venv "$(BACKEND_PYTHON_VENV)"; fi
	@cd "$(BACKEND_PYTHON_DIR)" && source "$(BACKEND_PYTHON_VENV)/bin/activate" && "$(BACKEND_PYTHON_PIP)" install --upgrade pip setuptools wheel
	@cd "$(BACKEND_PYTHON_DIR)" && source "$(BACKEND_PYTHON_VENV)/bin/activate" && "$(BACKEND_PYTHON_PIP)" install .

backend-python:
	@:

run-backend-python: setup-backend-python
	@cd "$(BACKEND_PYTHON_DIR)" && source "$(BACKEND_PYTHON_VENV)/bin/activate" && uvicorn app.main:app --reload

setup-frontend:
	@cd "$(FRONTEND_DIR)" && npm install

frontend:
	@:

run-frontend:
	@printf "NEXT_PUBLIC_API_BASE_URL=/api\nINTERNAL_API_BASE_URL=http://127.0.0.1:8000\nAPI_PROXY_TARGET=http://127.0.0.1:8000\n" > "$(FRONTEND_ENV_FILE)"
	@cd "$(FRONTEND_DIR)" && npm run dev
