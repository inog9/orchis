package injector

import (
	"context"
	"os"
	"sync"
	"time"

	"orchis-go/internal/defensebot"
	"orchis-go/internal/execution"
)

var (
	defenseOnce   sync.Once
	defenseOSV    *defensebot.OSV
	defenseEnrich *defensebot.Enricher
	defenseNVD    *defensebot.NVD
)

// defenseDeps returns shared clients so the advisory cache is reused between runs. Endpoints can be overridden for
// fixture-based smoke runs; production uses the public services.
func defenseDeps() defensebot.Deps {
	defenseOnce.Do(func() {
		defenseOSV = &defensebot.OSV{Base: os.Getenv("ORCHIS_DEFENSEBOT_OSV_URL"), Concurrency: 12}
		defenseEnrich = &defensebot.Enricher{KEVURL: os.Getenv("ORCHIS_DEFENSEBOT_KEV_URL"), EPSSURL: os.Getenv("ORCHIS_DEFENSEBOT_EPSS_URL")}
		// The NVD key is optional and free; it only raises the request limit. It lives in the worker's private file.
		defenseNVD = &defensebot.NVD{Base: os.Getenv("ORCHIS_DEFENSEBOT_NVD_URL"), APIKey: os.Getenv("ORCHIS_NVD_API_KEY")}
	})
	return defensebot.Deps{OSV: defenseOSV, NVD: defenseNVD, Enrich: defenseEnrich, Mapper: newAppCPEMapper()}
}

// executeDefensebot maps the stored software data of one asset to advisories. It only contacts the public vulnerability
// services, and only with package names and versions.
func (w *Worker) executeDefensebot(ctx context.Context, spec *execution.ClaimSpec) execution.Result {
	res := execution.Result{}
	if spec.Defense == nil {
		res.Error = "orchis-defense-agent input is missing."
		return res
	}
	timeout := time.Duration(spec.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go w.watch(runCtx, spec.ExecutionID, cancel, done)
	out := defensebot.Run(runCtx, *spec.Defense, defenseDeps())
	if ctx.Err() != nil || (runCtx.Err() != nil && runCtx.Err() != context.DeadlineExceeded) {
		res.Error = "The run was stopped."
		return res
	}
	for _, f := range out.Findings {
		res.Findings = append(res.Findings, execution.NewFinding{Type: f.Type, Value: f.Value, Title: f.Title, Severity: f.Severity, Details: f.Details})
	}
	for _, l := range out.Logs {
		res.Logs = append(res.Logs, execution.LogLine{Level: "info", Message: l})
	}
	code := 0
	res.ExitCode = &code
	res.Defense = &out.Summary
	return res
}
