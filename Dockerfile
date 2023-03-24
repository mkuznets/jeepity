FROM golang:1.19.5-alpine3.17 as build

ENV \
    CGO_ENABLED=1

RUN apk add --no-cache --update ca-certificates git gcc musl-dev bash curl make && \
    rm -rf /var/cache/apk/*

RUN go version && \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.50.1 && \
    golangci-lint --version && \
    mkdir -p "/build"

WORKDIR /build
ADD go.sum go.mod /build/
RUN go mod download
ADD sql/sqlite/dummy /build/dummy
RUN go build -ldflags='-extldflags "-static"' -o /tmp/dummy /build/dummy && \
    rm -rf /build/dummy

ADD . /build
RUN git config --global --add safe.directory /build && \
    make build

FROM alpine:3.17.2
RUN apk add --no-cache --update ca-certificates && \
    rm -rf /var/cache/apk/*

COPY --from=build /build/bin/jeepity /srv/jeepity
