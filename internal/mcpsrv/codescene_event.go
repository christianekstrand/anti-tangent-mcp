package mcpsrv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/patiently/anti-tangent-mcp/internal/codescene"
	"github.com/patiently/anti-tangent-mcp/internal/stats"
)

// codesceneRunKey identifies the run d reports by what the event file would
// hold for it, or returns "" when d reports no run. Two calls that send the
// same result share a key.
func codesceneRunKey(d *codescene.Digest) string {
	if d == nil || !d.Ran {
		return ""
	}
	b, err := json.Marshal(stats.RunRecord(*d))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// recordCodesceneRun writes the run d reports to the stats event file, unless
// d reports none or lastKey says the session's previous call already recorded
// this result: an implementer resends its CodeScene result on every
// validate_completion retry, and each retry is not a new run. It returns the
// key to keep on the session, "" when there is none to keep.
func (h *handlers) recordCodesceneRun(d *codescene.Digest, lastKey string) string {
	key := codesceneRunKey(d)
	if key == "" || key == lastKey {
		return key
	}
	h.deps.Stats.RecordCodescene(*d)
	return key
}
