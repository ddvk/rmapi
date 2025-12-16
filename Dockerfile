FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.6.1 AS xx

FROM --platform=$BUILDPLATFORM golang:alpine AS builder
COPY --from=xx / /
RUN apk add --no-cache git clang lld
ARG TARGETPLATFORM
RUN xx-apk add --no-cache musl-dev gcc

WORKDIR /src

# Copy go mod files first for dependency caching
COPY go.mod go.sum ./
# Download dependencies (this layer will be cached unless go.mod/go.sum change)
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
RUN xx-go --wrap && \
    CGO_ENABLED=0 xx-go build -ldflags="-s -w" -o rmapi .

FROM alpine:latest

RUN adduser -D app && \
    apk add --no-cache su-exec 

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

WORKDIR /home/app

COPY --from=builder /src/rmapi /usr/local/bin/rmapi

# Expose the REST API port
EXPOSE 8080

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["rmapi"] 
