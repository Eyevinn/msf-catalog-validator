BINARY := out/msf-catalog-validator

VERSION := $(shell git describe --tags HEAD 2>/dev/null || echo dev-$$(git rev-parse --short HEAD 2>/dev/null || echo unknown))
COMMIT_DATE := $(shell git log -1 --format=%ct 2>/dev/null)
LDFLAGS := -X github.com/Eyevinn/msf-catalog-validator/internal.commitVersion=$(VERSION) \
           -X github.com/Eyevinn/msf-catalog-validator/internal.commitDate=$(COMMIT_DATE)

.PHONY: all
all: test check coverage build

.PHONY: prepare
prepare:
	go mod tidy

.PHONY: build
build: prepare
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/msf-catalog-validator

.PHONY: test
test: prepare
	go test ./...

.PHONY: coverage
coverage:
	# Ignore (allow) packages without any tests
	go test ./... -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func coverage.out -o coverage.txt
	tail -1 coverage.txt

.PHONY: lint
lint: prepare
	golangci-lint run

.PHONY: venv
venv:
	python3 -m venv venv
	venv/bin/pip install --upgrade pip
	venv/bin/pip install pre-commit==4.2.0
	venv/bin/pip install codespell

.PHONY: pre-commit-install
pre-commit-install: venv
	venv/bin/pre-commit install

.PHONY: pre-commit
pre-commit: venv
	venv/bin/pre-commit run --all-files

.PHONY: codespell
codespell: venv
	venv/bin/codespell -S testdata,references,venv

.PHONY: check
check: prepare lint pre-commit codespell

.PHONY: update
update:
	go get -t -u ./...

# Validate a file: make run FILE=testdata/valid/msf_simulcast.json
.PHONY: run
run: build
	./$(BINARY) $(FILE)

.PHONY: serve
serve: build
	./$(BINARY) -serve :8080

.PHONY: clean
clean:
	rm -rf out coverage.out coverage.html coverage.txt
