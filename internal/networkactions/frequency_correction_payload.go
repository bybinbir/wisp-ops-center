package networkactions

import "sync"

// Phase 10E — typed payload side-channel for frequency_correction.
//
// The Action interface is generic across all Kinds and only carries
// the lowest-common-denominator Request fields (Kind, DeviceID,
// CorrelationID, DryRun, Confirm, Reason, Actor, Window). It does
// NOT carry a typed FrequencyCorrectionRequest because every other
// Kind would need its own typed shape and the interface would
// become unwieldy.
//
// Phase 10E threads the typed payload through a small in-memory
// registry keyed by CorrelationID. The handler:
//
//   1. Builds a FrequencyCorrectionRequest from the HTTP body.
//   2. Calls SetFrequencyCorrectionPayload(correlationID, &payload)
//      BEFORE dispatching to the runner.
//   3. After the runner returns (success or failure),
//      ClearFrequencyCorrectionPayload(correlationID) frees the slot.
//
// This is intentionally process-local: a destructive run is
// dispatched and consumed inside the same API process that received
// the HTTP request. We never persist the payload here — the typed
// fields live on the network_action_runs row (intent, target_host,
// idempotency_key) and in the audit metadata.

var (
	frequencyCorrectionPayloads   = map[string]*FrequencyCorrectionRequest{}
	frequencyCorrectionPayloadsMu sync.Mutex
)

// SetFrequencyCorrectionPayload stashes the typed payload for the
// runner to retrieve via getFrequencyCorrectionPayload. Caller MUST
// invoke ClearFrequencyCorrectionPayload after the runner returns
// (deferred is fine).
func SetFrequencyCorrectionPayload(correlationID string, p *FrequencyCorrectionRequest) {
	if correlationID == "" || p == nil {
		return
	}
	frequencyCorrectionPayloadsMu.Lock()
	defer frequencyCorrectionPayloadsMu.Unlock()
	frequencyCorrectionPayloads[correlationID] = p
}

// ClearFrequencyCorrectionPayload removes the typed payload from
// the side-channel. Idempotent.
func ClearFrequencyCorrectionPayload(correlationID string) {
	if correlationID == "" {
		return
	}
	frequencyCorrectionPayloadsMu.Lock()
	defer frequencyCorrectionPayloadsMu.Unlock()
	delete(frequencyCorrectionPayloads, correlationID)
}

// getFrequencyCorrectionPayload retrieves a stashed payload. Returns
// nil if no payload was set for the correlation_id.
func getFrequencyCorrectionPayload(correlationID string) *FrequencyCorrectionRequest {
	if correlationID == "" {
		return nil
	}
	frequencyCorrectionPayloadsMu.Lock()
	defer frequencyCorrectionPayloadsMu.Unlock()
	return frequencyCorrectionPayloads[correlationID]
}
