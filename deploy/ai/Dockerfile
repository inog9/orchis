FROM alpine:3.23 AS certificates
RUN apk add --no-cache ca-certificates
FROM scratch
COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY bin/ai-worker /usr/local/bin/ai-worker
COPY bin/ai-proxy /usr/local/bin/ai-proxy
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/ai-worker"]
