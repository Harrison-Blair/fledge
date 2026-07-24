// Package contextdoc validates analyzer artifacts and renders the generated
// project context document. The JSON types in this file are the wire contract
// shared by the agents that produce a context run and the Go CLI that consumes
// it.
package contextdoc

import "time"

const SchemaVersion = 1

type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type Scan struct {
	SchemaVersion int    `json:"schema_version"`
	Root          string `json:"root"`
	FileCount     int    `json:"file_count"`
	TotalSize     int64  `json:"total_size"`
	Files         []File `json:"files"`
}

// RenderResult identifies a successfully published project context document.
// ProvenancePath names the separately published provenance JSON object; it is
// empty when that follow-up write failed, which is reported as a warning.
// Warnings report best-effort durability or cleanup work that failed after the
// atomic publication point; they never mean the publication was rolled back.
type RenderResult struct {
	Path           string   `json:"path"`
	SHA256         string   `json:"sha256"`
	ProvenancePath string   `json:"provenance_path,omitempty"`
	Warnings       []string `json:"warnings"`
}

type AnalyzerRequest struct {
	SchemaVersion      int    `json:"schema_version"`
	GroupID            string `json:"group_id"`
	Purpose            string `json:"purpose"`
	InstructionsBefore string `json:"instructions_before,omitempty"`
	TotalSize          int64  `json:"total_size"`
	Files              []File `json:"files"`
	InstructionsAfter  string `json:"instructions_after,omitempty"`
}

type EntryPoint struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

type KeySymbol struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

type InternalDependency struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

type ExternalDependency struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Dependencies struct {
	Internal []InternalDependency `json:"internal"`
	External []ExternalDependency `json:"external"`
}

type DataFlow struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Description string `json:"description"`
}

type TestReference struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

type FileSummary struct {
	Path        string `json:"path"`
	ContentKind string `json:"content_kind"`
	Summary     string `json:"summary"`
}

// AnalyzerReply is the normalized form of either analyzer reply variant.
// Status selects its exact JSON shape: "ok" uses the analysis fields and
// "error" uses Errors.
type AnalyzerReply struct {
	SchemaVersion    int
	Status           string
	GroupID          string
	SubsystemSummary string
	EntryPoints      []EntryPoint
	KeySymbols       []KeySymbol
	Dependencies     Dependencies
	DataFlows        []DataFlow
	Invariants       []string
	Tests            []TestReference
	Files            []FileSummary
	Errors           []AnalyzerError
}

type AnalyzerError struct {
	Path    string `json:"path,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Routing struct {
	PathPrefix string `json:"path_prefix"`
	GroupID    string `json:"group_id"`
	Guidance   string `json:"guidance"`
}

type CrossGroupFlow struct {
	FromGroup   string `json:"from_group"`
	ToGroup     string `json:"to_group"`
	Description string `json:"description"`
}

type Synthesis struct {
	SchemaVersion    int              `json:"schema_version"`
	ProjectOverview  string           `json:"project_overview"`
	Routing          []Routing        `json:"routing"`
	CrossGroupFlows  []CrossGroupFlow `json:"cross_group_flows"`
	GlobalInvariants []string         `json:"global_invariants"`
}

type Identity struct {
	Name    string `json:"name"`
	Profile string `json:"profile"`
	Model   string `json:"model"`
}

type AnalyzerIdentity struct {
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	Profile string `json:"profile"`
	Model   string `json:"model"`
}

type Provenance struct {
	SchemaVersion int                `json:"schema_version"`
	Forager       Identity           `json:"forager"`
	Analyzers     []AnalyzerIdentity `json:"analyzers"`
	CreatedAt     *time.Time         `json:"created_at,omitempty"`
}

type AnalyzerSuccessReply struct {
	SchemaVersion    int             `json:"schema_version"`
	Status           string          `json:"status"`
	GroupID          string          `json:"group_id"`
	SubsystemSummary string          `json:"subsystem_summary"`
	EntryPoints      []EntryPoint    `json:"entry_points"`
	KeySymbols       []KeySymbol     `json:"key_symbols"`
	Dependencies     Dependencies    `json:"dependencies"`
	DataFlows        []DataFlow      `json:"data_flows"`
	Invariants       []string        `json:"invariants"`
	Tests            []TestReference `json:"tests"`
	Files            []FileSummary   `json:"files"`
}

type AnalyzerErrorReply struct {
	SchemaVersion int             `json:"schema_version"`
	Status        string          `json:"status"`
	GroupID       string          `json:"group_id"`
	Errors        []AnalyzerError `json:"errors"`
}
