FROM golang:1.26.10-alpine@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308 AS application
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
