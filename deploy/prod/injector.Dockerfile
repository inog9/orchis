ARG VERSION=local
FROM orchis-server:${VERSION} AS application
FROM debian:bookworm-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3
RUN apt-get update && apt-get install -y --no-install-recommends nmap=7.93+dfsg1-1 ca-certificates curl unzip \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --create-home --uid 10001 injector
# Official release archive and digest; no latest tag or silent missing-template fallback.
RUN curl -fsSL --connect-timeout 10 --max-time 600 -o /tmp/n.zip https://github.com/projectdiscovery/nuclei/releases/download/v3.11.1/nuclei_3.11.1_linux_amd64.zip \
 && echo 'ea63d4ae232808cd7c6bc00d0142428e231fab59dae01042246097d195835ab6  /tmp/n.zip' | sha256sum -c - \
 && unzip -q /tmp/n.zip nuclei -d /usr/local/bin && rm /tmp/n.zip
RUN curl -fsSL --connect-timeout 10 --max-time 300 -o /tmp/templates.zip https://codeload.github.com/projectdiscovery/nuclei-templates/zip/refs/tags/v10.5.1 \
 && echo '00184ad496302cd1dd3cf4b0b1c3aa45d6b35057f7bdf17e932318185ba0ac50  /tmp/templates.zip' | sha256sum -c - \
 && unzip -q /tmp/templates.zip -d /tmp/templates \
 && mkdir -p /home/injector/nuclei-templates/http \
 && cp -r /tmp/templates/nuclei-templates-10.5.1/http/exposures /tmp/templates/nuclei-templates-10.5.1/http/cves /tmp/templates/nuclei-templates-10.5.1/http/misconfiguration /home/injector/nuclei-templates/http/ \
 && cp /tmp/templates/nuclei-templates-10.5.1/LICENSE.md /home/injector/nuclei-templates/LICENSE.md \
 && cd /home/injector/nuclei-templates && find http -type f -exec sha256sum {} + | sort > SHA256SUMS \
 && rm -rf /tmp/templates /tmp/templates.zip \
 && apt-get purge -y unzip && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*
COPY deploy/prod/scanner-manifest.json /home/injector/nuclei-templates/manifest.json
COPY --from=application /usr/local/bin/injector /usr/local/bin/injector
ENV HOME=/tmp NUCLEI_TEMPLATES_DIR=/home/injector/nuclei-templates
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/injector"]
