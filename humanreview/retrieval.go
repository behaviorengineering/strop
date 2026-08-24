package humanreview

import "strings"

// ArtifactContentRetrievalKey is the artifact_content object that holds the retrieval snapshot.
const ArtifactContentRetrievalKey = "retrieval"

// RetrievalSnapshot is the portable pattern + novelty label on a learning artifact.
// Pack extras stay opaque JSON so kit does not import pipeline field names.
type RetrievalSnapshot struct {
	ObjectiveSummary string                 `json:"objective_summary"`
	DistinctiveMove  string                 `json:"distinctive_move"`
	DoNotReuseFor    string                 `json:"do_not_reuse_for,omitempty"`
	LoadBearing      string                 `json:"load_bearing,omitempty"`
	Extras           map[string]interface{} `json:"extras,omitempty"`
}

// SnapshotFromContent reads a retrieval snapshot from artifact_content.
// Missing or wrong-typed retrieval objects yield a zero snapshot (empty distinctive move).
func SnapshotFromContent(content map[string]interface{}) RetrievalSnapshot {
	if content == nil {
		return RetrievalSnapshot{}
	}
	raw, ok := content[ArtifactContentRetrievalKey]
	if !ok {
		return RetrievalSnapshot{}
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return RetrievalSnapshot{}
	}
	return RetrievalSnapshot{
		ObjectiveSummary: stringField(obj, "objective_summary"),
		DistinctiveMove:  stringField(obj, "distinctive_move"),
		DoNotReuseFor:    stringField(obj, "do_not_reuse_for"),
		LoadBearing:      stringField(obj, "load_bearing"),
		Extras:           mapField(obj, "extras"),
	}
}

// PutSnapshot writes snapshot into artifact_content under ArtifactContentRetrievalKey.
func PutSnapshot(content map[string]interface{}, snapshot RetrievalSnapshot) map[string]interface{} {
	if content == nil {
		content = map[string]interface{}{}
	}
	content[ArtifactContentRetrievalKey] = snapshot.asMap()
	return content
}

func (s RetrievalSnapshot) asMap() map[string]interface{} {
	out := map[string]interface{}{
		"objective_summary": s.ObjectiveSummary,
		"distinctive_move":  s.DistinctiveMove,
	}
	if s.DoNotReuseFor != "" {
		out["do_not_reuse_for"] = s.DoNotReuseFor
	}
	if s.LoadBearing != "" {
		out["load_bearing"] = s.LoadBearing
	}
	if len(s.Extras) > 0 {
		out["extras"] = s.Extras
	}
	return out
}

func stringField(obj map[string]interface{}, key string) string {
	value, ok := obj[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func mapField(obj map[string]interface{}, key string) map[string]interface{} {
	value, ok := obj[key].(map[string]interface{})
	if !ok {
		return nil
	}
	return value
}
