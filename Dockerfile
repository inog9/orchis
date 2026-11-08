FROM orchis-hackbot-engine:orchis-v1
USER root
RUN npm install -g @opencode/cli@2.0.11 && npm cache clean --force
COPY src /opt/orchis-opencode/src
RUN rm /usr/local/bin/pentest-job && ln -s /opt/orchis-opencode/src/job.mjs /usr/local/bin/pentest-job && chmod 755 /opt/orchis-opencode/src/job.mjs
USER pentest
