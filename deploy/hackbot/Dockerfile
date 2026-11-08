FROM golang:1.27.3-bookworm AS application
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
RUN CGO_ENABLED=0 go build -trimpath -o /out/injector ./cmd/injector
# Built locally from the locked tool and OpenCode worker sources.
FROM orchis-opencode-engine:orchis-v1
USER root
COPY --from=application /out/injector /usr/local/bin/injector
RUN test -x /usr/local/bin/pentest-job && opencode --version
USER pentest
CMD ["injector", "--kind", "hackbot"]
