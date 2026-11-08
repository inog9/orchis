ARG VERSION=local
FROM orchis-server:${VERSION} AS application
# Built locally from the locked tool and OpenCode worker sources.
FROM orchis-opencode-engine:orchis-v1
USER root
COPY --from=application /usr/local/bin/injector /usr/local/bin/injector
RUN test -x /usr/local/bin/pentest-job && opencode --version
USER pentest
CMD ["injector", "--kind", "hackbot"]
