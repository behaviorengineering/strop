// Package agentsession stores short-lived agent conversations as one directory
// per session under an app-injected Root. It does not run LLMs or tools.
package agentsession

import "time"

// Status values for Meta.Status.
const (
	StatusOpen   = "open"
	StatusClosed = "closed"
)

// Well-known blob filenames apps may use with SaveJSON / LoadJSON.
const (
	FileMeta       = "meta.json"
	FileTranscript = "transcript.jsonl"
	FileEvidence   = "evidence.json"
	FileCard       = "card.json"
	FileTrajectory = "trajectory.json"
	FileFailure    = "failure.json"
)

// Meta is the durable session header (meta.json).
// Extra holds app-specific fields (project_id, branch, …) without strop knowing them.
type Meta struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind,omitempty"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// Turn is one line in transcript.jsonl.
type Turn struct {
	At      time.Time      `json:"at"`
	Role    string         `json:"role"` // user | assistant | system | tool
	Content string         `json:"content,omitempty"`
	Refs    map[string]any `json:"refs,omitempty"`
}

// ListOpts filters List results. Empty Kind / Status match all.
type ListOpts struct {
	Kind   string
	Status string
}

// PruneOpts controls directory cleanup under sessions/.
type PruneOpts struct {
	// MaxAge deletes sessions whose UpdatedAt (or dir mtime) is older than this.
	// Zero means do not age-prune.
	MaxAge time.Duration
	// KeepPerKind keeps the newest N sessions per Kind after age prune.
	// Zero means do not trim by count.
	KeepPerKind int
	// ClosedOnly limits prune to status=closed when true.
	ClosedOnly bool
}
