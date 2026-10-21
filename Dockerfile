# ownership_board: one static Go binary with the web app embedded.

# ---- web app -----------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- server ------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /web/dist ./internal/webui/dist
ARG VERSION=dev
# Static and stripped: no CGO (the SQLite driver is pure Go), reproducible paths.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags embed_frontend \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/ownership ./cmd/ownership \
 && mkdir -p /out/data

# ---- runtime -----------------------------------------------------------------
# Distroless: no shell, no package manager, non-root. The binary checks its own
# health because there is no curl.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ownership /ownership
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV DATA_DIR=/data PORT=8080
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/ownership", "healthcheck"]
ENTRYPOINT ["/ownership"]
