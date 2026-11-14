package scans

// A delivered final pentest report completes the run, even when some coverage
// was blocked. Keep the stored status and report outcomes for coverage decisions.
const scanCompletionSQL = `CASE WHEN a.status='incomplete' AND a.results_expired_at IS NULL
 AND EXISTS(SELECT 1 FROM scan_jobs web WHERE web.tenant_id=a.tenant_id AND web.assessment_id=a.id AND web.tool='hackbot' AND web.status='incomplete')
 AND NOT EXISTS(SELECT 1 FROM scan_jobs pending WHERE pending.tenant_id=a.tenant_id AND pending.assessment_id=a.id AND pending.status<>'finished' AND NOT (
  pending.tool='hackbot' AND pending.status='incomplete' AND EXISTS(
   SELECT 1 FROM executions delivered WHERE delivered.tenant_id=pending.tenant_id AND delivered.simulation_id=pending.simulation_id AND delivered.state='ended'
    AND delivered.outcome NOT IN ('canceled','timeout') AND delivered.hackbot_result->>'phase'='report'
    AND delivered.hackbot_result->>'disposition' IN ('limited','input_required')
    AND COALESCE(delivered.hackbot_result->>'report','')<>'' AND COALESCE(delivered.hackbot_result->>'validation','')<>''
  )
 )) THEN 'finished' ELSE '' END`
