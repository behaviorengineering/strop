package runreport

// Config controls JSON execution trace files for pipeline debugging.
// Apps map their own config (e.g. YAML run_reports) into this type at the boundary.
type Config struct {
	Enabled           bool
	Dir               string
	MaxAgeHours       int
	KeepPerEntity     int
	RecordModuleCalls bool
}

// Defaults returns safe defaults when config is omitted.
func (c Config) Defaults() Config {
	out := c
	if out.Dir == "" {
		out.Dir = "logs/runs"
	}
	if out.MaxAgeHours <= 0 {
		out.MaxAgeHours = 48
	}
	if out.KeepPerEntity <= 0 {
		out.KeepPerEntity = 2
	}
	return out
}
