package main

type loadReportV1 struct {
	SchemaVersion int    `json:"schema_version"`
	RunID         string `json:"run_id"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	Target        struct {
		BaseURL  string `json:"base_url"`
		TenantID string `json:"tenant_id"`
	} `json:"target"`
	Profile struct {
		Name            string `json:"name"`
		VUs             int    `json:"vus"`
		DurationSeconds int    `json:"duration_seconds"`
	} `json:"profile"`
	Backend struct {
		DeepReadBackend string `json:"deep_read_backend"`
		CacheEnabled    bool   `json:"cache_enabled"`
	} `json:"backend"`
	Results    []loadReportResult    `json:"results"`
	Thresholds []loadReportThreshold `json:"thresholds"`
	Notes      string                `json:"notes"`
}

type loadReportResult struct {
	Endpoint string `json:"endpoint"`
	Count    int    `json:"count"`
	Errors   int    `json:"errors"`
	P50MS    int    `json:"p50_ms"`
	P95MS    int    `json:"p95_ms"`
	P99MS    int    `json:"p99_ms"`
}

type loadReportThreshold struct {
	Name  string `json:"name"`
	Limit int    `json:"limit"`
	OK    bool   `json:"ok"`
}
