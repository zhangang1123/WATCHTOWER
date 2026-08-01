package models

// Diagnosis Agent 诊断结果
type Diagnosis struct {
	IncidentID    string          `json:"incident_id"`
	RootCause     string          `json:"root_cause"`
	Confidence    float64         `json:"confidence"` // 0~1
	Evidence      []Evidence      `json:"evidence"`
	SuggestedFix  string          `json:"suggested_fix"`
	FixType       string          `json:"fix_type"`
	AutoFixable   bool            `json:"auto_fixable"`
	DiagnosisPath []DiagnosisStep `json:"diagnosis_path"`
	Escalated     bool            `json:"escalated"`
	Summary       string          `json:"summary"`
}

// Evidence 诊断证据
type Evidence struct {
	Source      string `json:"source"` // prometheus / k8s / logs
	Description string `json:"description"`
	RawData     string `json:"raw_data"`
}

// DiagnosisStep 诊断步骤
type DiagnosisStep struct {
	StepNumber  int64  `json:"step_number"`
	Thought     string `json:"thought"`
	Action      string `json:"action"`
	Observation string `json:"observation"`
	LatencyMs   int64  `json:"latency_ms"`
}
