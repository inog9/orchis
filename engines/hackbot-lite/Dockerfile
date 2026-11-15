# syntax=docker/dockerfile:1
FROM golang:1.25.4-bookworm AS go
FROM oven/bun:1.3.14 AS bun

# Current gog, httpx and interactsh need Go 1.26+. Keep their newer compiler
# isolated from the harness/runtime compiler and copy only finished tools.
FROM golang:1.27.3-bookworm AS tools-build
ENV GOBIN=/out/bin GOFLAGS=-p=4
COPY docker/install-go-tools.sh /tmp/install-go-tools.sh
RUN sh /tmp/install-go-tools.sh && go clean -cache -modcache

FROM node:22.21.1-bookworm AS jxscout-build
COPY --from=go /usr/local/go /usr/local/go
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
ENV PATH="/usr/local/go/bin:${PATH}"
ARG JXSCOUT_REV=c43dbd56139a6d332bcc1bfba502f1736b7a3185
RUN apt-get update && apt-get install -y --no-install-recommends python3 && rm -rf /var/lib/apt/lists/*
WORKDIR /src/jxscout
RUN curl -fL --retry 3 "https://codeload.github.com/r1pu5u/jxscout/tar.gz/${JXSCOUT_REV}" | tar xz --strip-components=1
COPY docker/patch-jxscout.py /tmp/patch-jxscout.py
RUN python3 /tmp/patch-jxscout.py /src/jxscout && bun install --frozen-lockfile && bun run scripts/build-agent-cli.ts \
    && gofmt -w internal/core/asset-fetcher/service.go \
    && go build -mod=vendor -o /out/jxscout ./cmd/jxscout \
    && go clean -cache && rm -rf node_modules /root/.bun

FROM go AS harness-build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/pentest-harnes ./cmd/pentest-harnes \
    && CGO_ENABLED=0 go build -o /out/gori-jxscout ./cmd/gori-jxscout && go clean -cache

FROM node:22.21.1-bookworm
ARG TARGETARCH
COPY --from=go /usr/local/go /usr/local/go
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
ENV PATH="/usr/local/go/bin:${PATH}" \
    PENTEST_HARNES_STACK=/opt/pentest/docker/stack.py
RUN apt-get update && apt-get install -y --no-install-recommends \
      chromium libnss3-tools ca-certificates python3 python3-publicsuffix2 curl git \
      jq ripgrep unzip zip xz-utils file openssl bash coreutils findutils diffutils \
      procps gawk sed grep tar gzip bzip2 \
      libssl3 libsqlite3-0 libpcre2-8-0 libevent-2.1-7 libgc1 libbrotli1 libzstd1 \
    && rm -rf /var/lib/apt/lists/*
COPY docker/download-tools.sh /tmp/download-tools.sh
RUN sh /tmp/download-tools.sh "$TARGETARCH"
RUN npm install -g @earendil-works/pi-coding-agent@1.5.0 prettier@3.9.10 webcrack@2.16.0 \
    && npm install --prefix /opt/pi-packages pi-subagents@0.86.0 billion-context@0.1.212 @xynogen/pix-todo@0.4.13 @juicesharp/rpiv-ask-user-question@2.15.0 \
    && npm cache clean --force
COPY docker/install-npxconfuse.sh /tmp/install-npxconfuse.sh
RUN sh /tmp/install-npxconfuse.sh && npm cache clean --force
COPY --from=tools-build /out/bin/ /usr/local/bin/
COPY --from=harness-build /out/ /usr/local/bin/
COPY --from=jxscout-build /out/jxscout /usr/local/bin/jxscout
COPY docker /opt/pentest/docker
COPY docker/pi-settings.json /opt/pentest/pi-settings.json
RUN useradd -m -u 1001 pentest && mkdir /data && chown pentest:pentest /data \
    && chmod +x /opt/pentest/docker/entrypoint.sh /opt/pentest/docker/run-job.mjs \
    && ln -s /opt/pentest/docker/run-job.mjs /usr/local/bin/pentest-job
USER pentest
WORKDIR /data
RUN python3 /opt/pentest/docker/tool-smoke.py
ENTRYPOINT ["/opt/pentest/docker/entrypoint.sh"]
CMD ["pentest-harnes"]
