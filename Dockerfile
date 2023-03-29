FROM ghcr.io/mkuznets/build-go:1.20.2-20230328234842 as build

ENV \
    CGO_ENABLED=1

RUN go version && \
    mkdir -p "/build"

WORKDIR /build
ADD go.sum go.mod /build/
RUN go mod download

ADD . /build
RUN git config --global --add safe.directory /build && \
    make build

FROM alpine:3.17.2
RUN apk add --no-cache --update ca-certificates && \
    rm -rf /var/cache/apk/*

COPY --from=build /build/bin/jeepity /srv/jeepity
