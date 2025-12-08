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

RUN adduser -D app && \
    apk add --no-cache su-exec && \
    mkdir -p /home/app/.config/rmapi && \
    mkdir -p /home/app/.cache/rmapi && \
    mkdir -p /home/app/downloads && \
    chown -R app:app /home/app/.config && \
    chown -R app:app /home/app/.cache && \
    chown -R app:app /home/app/downloads

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

WORKDIR /home/app/downloads

COPY --from=builder /src/rmapi /usr/local/bin/rmapi

# Expose the REST API port
EXPOSE 8080

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["rmapi"] 
