COMMIT := $(shell if [ -d .git ]; then git rev-parse HEAD; else echo "unknown"; fi)
SHORTCOMMIT := $(shell echo $(COMMIT) | head -c 7)
BRANCH := $(shell if [ -d .git ]; then git rev-parse --abbrev-ref HEAD; else echo "master"; fi)
BUILD := $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")
BINSUFFIX := $(shell if [ "${GOOS}" -a "${GOARCH}" ]; then echo "-${GOOS}-${GOARCH}"; else echo ""; fi)

all: build

## build: Build core (default)
build:
	CGO_ENABLED=${CGO_ENABLED} GOOS=${GOOS} GOARCH=${GOARCH} go build -o core${BINSUFFIX}

# github workflow workaround
build_linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=${OSARCH} go build -o core

## swagger: Update swagger API documentation (requires github.com/swaggo/swag)
swagger:
	swag init -g http/server.go

## gqlgen: Regenerate GraphQL server from schema
gqlgen:
	go run github.com/99designs/gqlgen generate --config http/graph/gqlgen.yml

## test: Run all tests
test:
	go test -race -coverprofile=/dev/null -v ./...

## vet: Analyze code for potential errors
vet:
	go vet ./...

## fmt: Format code
fmt:
	go fmt ./...

## update: Update dependencies
update:
	go get -u
	@-$(MAKE) tidy
	@-$(MAKE) vendor

## tidy: Tidy up go.mod
tidy:
	go mod tidy

## vendor: Update vendored packages
vendor:
	go mod vendor

## run: Build and run core
run: build
	./core

## lint: Static analysis with staticcheck
lint:
	staticcheck ./...

## import: Build import binary
import:
	cd app/import && CGO_ENABLED=${CGO_ENABLED} GOOS=${GOOS} GOARCH=${GOARCH} go build -o ../../import -ldflags="-s -w"

# github workflow workaround
import_linux:
	cd app/import && CGO_ENABLED=0 GOOS=linux GOARCH=${OSARCH} go build -o ../../import -ldflags="-s -w"

## coverage: Generate code coverage analysis
coverage:
	go test -race -coverprofile test/cover.out ./...
	go tool cover -html=test/cover.out -o test/cover.html

## commit: Prepare code for commit (vet, fmt, test)
commit: vet fmt lint test build
	@echo "No errors found. Ready for a commit."

## release: Build a release binary of core
release:
	CGO_ENABLED=${CGO_ENABLED} GOOS=${GOOS} GOARCH=${GOARCH} go build -o core -ldflags="-s -w -X github.com/datarhei/core/v16/app.Commit=$(COMMIT) -X github.com/datarhei/core/v16/app.Branch=$(BRANCH) -X github.com/datarhei/core/v16/app.Build=$(BUILD)"

# github workflow workaround
release_linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=${OSARCH} go build -o core -ldflags="-s -w -X github.com/datarhei/core/v16/app.Commit=$(COMMIT) -X github.com/datarhei/core/v16/app.Branch=$(BRANCH) -X github.com/datarhei/core/v16/app.Build=$(BUILD)"

## docker: Build standard Docker image
docker:
	docker build -t core:$(SHORTCOMMIT) .

.PHONY: help build swagger test vet fmt vendor commit coverage lint release import update

## help: Show all commands
help: Makefile
	@echo
	@echo " Choose a command:"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo
