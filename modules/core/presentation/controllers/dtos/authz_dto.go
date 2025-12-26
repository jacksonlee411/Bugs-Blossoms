package dtos

// APIError standardizes JSON error responses.
type APIError struct {
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// PolicyEntryResponse mirrors services.PolicyEntry for JSON responses.
type PolicyEntryResponse struct {
	Type    string `json:"type"`
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object"`
	Action  string `json:"action"`
	Effect  string `json:"effect"`
}

// PolicyListResponse wraps paginated policies.
type PolicyListResponse struct {
	Data  []PolicyEntryResponse `json:"data"`
	Total int                   `json:"total"`
	Page  int                   `json:"page"`
	Limit int                   `json:"limit"`
}

// StagePolicyRequest captures payloads for staging a policy diff.
type StagePolicyRequest struct {
	Type      string `json:"type"`
	Subject   string `json:"subject"`
	Domain    string `json:"domain"`
	Object    string `json:"object"`
	Action    string `json:"action"`
	Effect    string `json:"effect"`
	StageKind string `json:"stage_kind,omitempty"`
}

// StagedPolicyEntry represents a staged policy change with a client-side id.
type StagedPolicyEntry struct {
	ID        string `json:"id"`
	StageKind string `json:"stage_kind,omitempty"`
	PolicyEntryResponse
}

// StagePolicyResponse returns the current staged entries.
type StagePolicyResponse struct {
	Data       []StagedPolicyEntry `json:"data"`
	Total      int                 `json:"total"`
	CreatedIDs []string            `json:"created_ids,omitempty"`
}

// DebugResponse provides a detailed Authz.Debug output.
type DebugResponse struct {
	Allowed       bool            `json:"allowed"`
	Mode          string          `json:"mode"`
	LatencyMillis int64           `json:"latency_ms"`
	Request       DebugRequestDTO `json:"request"`
	Attributes    map[string]any  `json:"attributes,omitempty"`
	Trace         DebugTraceDTO   `json:"trace"`
}

// DebugTraceDTO contains rule chain information for Authz.Debug.
type DebugTraceDTO struct {
	MatchedPolicy []string `json:"matched_policy,omitempty"`
}

// DebugRequestDTO echoes the evaluated request payload.
type DebugRequestDTO struct {
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object"`
	Action  string `json:"action"`
}
