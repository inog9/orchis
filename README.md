# Orchis OpenCode worker

Runs one OpenCode agent per Orchis engagement after Nmap and Nuclei. The worker installs the bundled pentest skills with `pentest-harnes init`, starts Gori, Interceptor and JXScout, and passes a structured result to Orchis. Orchis stores the result and renders its PDF. The OpenCode model defaults to `opencode-go/deepseek-v4.1-flash`; subagents are disabled and the model run has a 25 minute limit.

The image currently builds on the existing `orchis-hackbot-engine:orchis-v1` tool image. That image supplies the pentest harness, Gori, browser and JXScout and contains the older Pi installation; this worker does not run Pi. OpenCode Go credentials are read from the existing private `/home/pentest/.pi/agent/auth.json` credential store. Existing OpenCode MCP servers may also be configured in the persistent home.

Build with `docker build -t orchis-opencode-engine:orchis-v1 .` after building the pinned tool image. Run `npm test` for the result validation tests. Only findings with private evidence files are passed on; an evidence file alone does not independently prove a finding.
