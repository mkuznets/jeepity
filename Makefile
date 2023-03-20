LDFLAGS := -s -w
OS=$(shell uname -s)

ifneq ($(OS),Darwin)
	LDFLAGS += -extldflags "-static"
endif

.PHONY: build
build: jeepity

.PHONY: jeepity
jeepity:
	mkdir -p bin
	export CGO_ENABLED=1
	go build -p 16 -ldflags='${LDFLAGS}' -o bin/jeepity mkuznets.com/go/jeepity/cmd/jeepity

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: run
run: jeepity
	mkdir -p data
	bin/jeepity run

.PHONY: test
test:
	go test -v ./...

.PHONY: distclean
distclean:
	rm -rf bin data

.PHONY: precommit
precommit:
	make fmt tidy test
