# 前端 dist 由 CLIENT_WEB_URL 在 go build 前注入 embed 目录
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS go-builder
WORKDIR /src
ARG TARGETARCH
ARG CLIENT_WEB_URL
ARG CLIENT_WEB_SHA256
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
ADD $CLIENT_WEB_URL /tmp/web-dist.tar.gz
RUN echo "$CLIENT_WEB_SHA256  /tmp/web-dist.tar.gz" | sha256sum -c - \
 && mkdir -p ./internal/webui/dist \
 && tar -xzf /tmp/web-dist.tar.gz --strip-components=1 -C ./internal/webui/dist \
 && test -f ./internal/webui/dist/index.html
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /gospeak main.go
FROM alpine:3.21
ARG VERSION=dev
ARG COMMIT=unknown
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=go-builder /gospeak /app/gospeak
RUN printf '%s\n' "${VERSION}" > /app/VERSION \
    && printf '%s\n' "${COMMIT}" > /app/COMMIT
RUN mkdir -p /app/db /app/logs /app/uploads /app/nats \
    && chown -R app:app /app

ENV PATH=/app/plugins/runtime/bin:$PATH
ENV GIN_MODE=release
ENV SERVER_PORT=8998
ENV DB_PATH=/app/db/app.db
ENV NATS_STORE_DIR=/app/nats
EXPOSE 8998
VOLUME ["/app/db", "/app/logs", "/app/uploads", "/app/nats"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8998/ping || exit 1
USER app
ENTRYPOINT ["/app/gospeak", "server", "-e", "prod"]
