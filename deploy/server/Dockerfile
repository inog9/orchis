# Build all application artifacts inside the image; no host Go/Node installation needed.
FROM node:24.22.0-alpine3.24@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

FROM golang:1.26.10-alpine@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
COPY deploy/mitre/ deploy/mitre/
# Unduhan hanya saat build jika artefak lokal belum ada; manifest tetap menjadi sumber pin.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go run ./cmd/mitre-update -fetch \
 && go run ./cmd/mitre-update -verify \
 && chmod 0555 deploy/mitre \
 && chmod 0444 deploy/mitre/*.json
ARG VERSION=local
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/ai-worker ./cmd/ai-worker \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ai-proxy ./cmd/ai-proxy \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/injector ./cmd/injector \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/backup-seal ./cmd/backup-seal

FROM scratch AS runtime
COPY --from=go /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go /out/ /usr/local/bin/
COPY --from=web /src/web/dist/ /web/
COPY --from=go /src/deploy/mitre/ /opt/orchis/mitre/
COPY docs/licenses/ /licenses/
COPY NOTICE /licenses/NOTICE
USER 10001:10001
ENV ORCHIS_HTTP_ADDR=0.0.0.0:8090 ORCHIS_WEB_DIR=/web
EXPOSE 8090
HEALTHCHECK --interval=15s --timeout=5s CMD ["/usr/local/bin/server", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/server"]
CMD ["serve"]
