FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.6.1 AS xx

FROM --platform=$BUILDPLATFORM golang:alpine AS builder
COPY --from=xx / /
RUN apk add --no-cache git clang lld
ARG TARGETPLATFORM
RUN xx-apk add --no-cache musl-dev gcc

WORKDIR /src
COPY . .
RUN xx-go --wrap && \
    CGO_ENABLED=0 xx-go build -ldflags="-s -w" -o rmapi .

FROM alpine:latest

RUN adduser -D app
USER app

COPY --from=builder /src/rmapi /usr/local/bin/rmapi
ENTRYPOINT ["rmapi"] 
