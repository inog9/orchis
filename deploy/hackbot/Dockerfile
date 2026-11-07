ARG VERSION=local
FROM orchis-server:${VERSION} AS application
# Built locally from the locked Hackbot source plus the result-export patch.
FROM orchis-hackbot-engine:orchis-v1
USER root
COPY --from=application /usr/local/bin/injector /usr/local/bin/injector
RUN test -f /opt/pentest/docker/export-result.mjs && test -f /opt/pentest/docker/scanner-inputs.mjs
USER pentest
CMD ["injector", "--kind", "hackbot"]
