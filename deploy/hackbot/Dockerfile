FROM golang:1.26.11-alpine@sha256:ae52b5bc6ed045a9ed26ab03dc19327a802a9cc9aa8b27ed57f5c6b60579b0bb AS application
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
ARG VERSION=local
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/injector ./cmd/injector
# Built locally from the locked tool and OpenCode worker sources.
FROM orchis-opencode-engine:orchis-v1
USER root
COPY --from=application /out/injector /usr/local/bin/injector
RUN test -x /usr/local/bin/pentest-job && opencode --version
USER pentest
CMD ["injector", "--kind", "hackbot"]
