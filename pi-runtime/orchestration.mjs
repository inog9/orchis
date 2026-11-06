// Pi's native async worker completion wakes the lead; sleeping in a lead shell
// while workers run wastes model turns and delays useful evidence verification.
export function isIdleShellPolling(toolName, input, runningWorkers) {
  if (!runningWorkers || toolName !== "bash" || typeof input?.command !== "string") return false;
  return /^\s*sleep\s+\d+(?:\.\d+)?\s*(?:$|;|&&)/.test(input.command);
}
