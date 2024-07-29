VERSION=0.1.0

PWD ?= `pwd`

# Installs buf.build on OSX if it doesn't already exist
.PHONY: dev-setup
dev-setup-osx:
	brew update
	brew list bufbuild/buf/buf || brew install bufbuild/buf/buf

# Tidies up go modules
.PHONY: tidy
tidy:
	go mod tidy

VERSION := 0.1.0

# Jogger has to run on Linux. For OSX users, this is an ubuntu image
# used for local development
.PHONY: docker-build
docker-build:
	docker build \
    		-f Dockerfile \
    		-t jogger-server-amd64:$(VERSION) \
    		--build-arg BUILD_REF=$(VERSION) \
    		--build-arg BUILD_DATE=`date -u +"%Y-%m-%dT%H:%M:%SZ"` \
    		.

# Run the local development ubuntu image
.PHONY: docker-run
docker-run: docker-build
	docker run jogger-server-amd64:$(VERSION)

# Run the local development ubuntu image with a shell
.PHONY: docker-shell
docker-shell: docker-build
	docker run -it jogger-server-amd64:$(VERSION) /bin/sh

# Generate the protobuf and grpc code
.PHONY: grpc
grpc:
	buf generate

# Lint the project
.PHONY: lint
lint:
	buf lint
	go vet ./...

# Run the Jogger server
.PHONY: run-server
run-server:
	go run ./cmd/server/main.go | go run ./cmd/tools/logfmt/main.go

# Build the Jogger CLI `jog`
.PHONY: build-cli
build-cli:
	go build -o jog ./cmd/jog/main.go

# Install the Jogger CLI `jog` in the $GOBIN directory.
# Note that it is assumed that $GOBIN is in the $PATH
.PHONY: install-cli
install-cli: build-cli
	mv jog $(GOBIN)/jog

# Generate the certs for the server and client
.PHONY: gen-certs
gen-certs:
	go run ./cmd/tools/gencerts/main.go

# Run the tests
.PHONY: test
test:
	go test ./...
