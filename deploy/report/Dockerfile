FROM mcr.microsoft.com/playwright:v1.64.1-noble@sha256:47b510dd9230bf0b9bfc9b20102241e577ea7a99334ab4c3ca19f19770e9b8ea
WORKDIR /app
COPY deploy/report/package*.json ./
RUN npm ci --omit=dev --ignore-scripts && npm cache clean --force
COPY deploy/report/render.mjs deploy/report/serve.mjs ./
USER pwuser
EXPOSE 8091
HEALTHCHECK --interval=15s --timeout=5s CMD node -e "fetch('http://127.0.0.1:8091/healthz').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"
CMD ["node", "serve.mjs"]
