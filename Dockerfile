FROM golang:1.18.1-alpine AS builder

ARG BUILD_VERSION=0.0.0
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG PROGNAME=publisher

RUN mkdir -p -v /src
WORKDIR /src
ADD . /src

RUN apk add git
RUN GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go get
RUN GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -ldflags="-X 'main.BuildVersion=${BUILD_VERSION}'" -v -o "${PROGNAME}" .


FROM alpine:3.15

ARG LISTEN_ADDRESS="0.0.0.0"
ARG LISTEN_PORT="8080"

ARG REDIS_ADDRESS="127.0.0.1"
ARG REDIS_PORT="6379"

COPY --from=builder /src/publisher /publisher

CMD ["-verbose", "-bind", "${LISTEN_ADDRESS}:${LISTEN_PORT}", "-redis-address", "${REDIS_ADDRESS}:${REDIS_PORT}"]
ENTRYPOINT ["./publisher"]
