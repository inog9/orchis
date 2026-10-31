# Injector orchis-go: satu image untuk nmap dan nuclei (pilih lewat --kind).
# Bangun biner dulu di host:  CGO_ENABLED=0 go build -o bin/injector ./cmd/injector
# Lalu:                        docker build -f deploy/injector/Dockerfile -t orchis-injector:dev .
FROM debian:bookworm-slim
ARG NUCLEI_VERSION=3.11.1
RUN apt-get update \
 && apt-get install -y --no-install-recommends nmap ca-certificates curl unzip \
 && curl -fsSL -o /tmp/n.zip "https://github.com/projectdiscovery/nuclei/releases/download/v${NUCLEI_VERSION}/nuclei_${NUCLEI_VERSION}_linux_amd64.zip" \
 && unzip -q /tmp/n.zip nuclei -d /usr/local/bin && rm /tmp/n.zip \
 && apt-get purge -y unzip && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*
# Template resmi diunduh saat build agar container tidak memerlukan internet untuk memulai.
RUN useradd --create-home --uid 10001 injector
USER injector
ENV HOME=/home/injector NUCLEI_TEMPLATES_DIR=/home/injector/nuclei-templates
RUN nuclei -update-templates -ud /home/injector/nuclei-templates -silent || echo "template resmi tidak terunduh; gunakan NUCLEI_TEMPLATES_DIR"
COPY --chown=injector bin/injector /usr/local/bin/injector
ENTRYPOINT ["/usr/local/bin/injector"]
