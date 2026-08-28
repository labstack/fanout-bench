set dotenv-load := true

telemetrygen_version := "v0.157.0"

default: test

build:
    mkdir -p bin
    go build -trimpath -ldflags "-s -w -X main.version=$(git describe --always --dirty 2>/dev/null || echo dev)" -o bin/fanout-bench ./cmd/fanout-bench

telemetrygen:
    mkdir -p bin
    GOBIN="$(pwd)/bin" go install github.com/open-telemetry/opentelemetry-collector-contrib/cmd/telemetrygen@{{telemetrygen_version}}

test:
    go test ./...
    go vet ./...

smoke: build telemetrygen
    ./bin/fanout-bench run --preset quick --telemetrygen ./bin/telemetrygen

