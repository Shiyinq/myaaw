package computer

// ProbeResult is one environment check produced by Probe.
type ProbeResult struct {
	Name   string // short check name
	OK     bool
	Detail string // human-readable status or the raw error
	Fix    string // remediation hint when !OK (may be empty)
}
