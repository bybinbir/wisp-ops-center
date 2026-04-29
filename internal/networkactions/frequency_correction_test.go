package networkactions

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeWriter is a deterministic FrequencyCorrectionWriter for unit
// tests. Each call increments a counter so tests can assert "we
// wrote N times" and "we read N times" without an SSH stack.
type fakeWriter struct {
	mu sync.Mutex

	// Programmable behaviour:
	snapshotSeq []snapshotResult // consumed in order; pop on each call
	setSeq      []error          // consumed in order; pop on each call

	// Recorded calls (for assertions):
	snapshots []string    // device IDs we read from
	sets      []setRecord // device + iface + mhz we wrote
}

type snapshotResult struct {
	mhz int
	err error
}

type setRecord struct {
	deviceID string
	iface    string
	mhz      int
}

func (w *fakeWriter) SnapshotFrequency(_ context.Context, deviceID, _ string) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.snapshots = append(w.snapshots, deviceID)
	if len(w.snapshotSeq) == 0 {
		return 0, errors.New("test: snapshotSeq exhausted")
	}
	r := w.snapshotSeq[0]
	w.snapshotSeq = w.snapshotSeq[1:]
	return r.mhz, r.err
}

func (w *fakeWriter) SetFrequency(_ context.Context, deviceID, iface string, mhz int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sets = append(w.sets, setRecord{deviceID: deviceID, iface: iface, mhz: mhz})
	if len(w.setSeq) == 0 {
		return errors.New("test: setSeq exhausted")
	}
	e := w.setSeq[0]
	w.setSeq = w.setSeq[1:]
	return e
}

// fakeAudit captures every emit so tests can assert order + metadata.
type fakeAudit struct {
	mu     sync.Mutex
	events []auditEvent
}

type auditEvent struct {
	action   DestructiveAuditAction
	outcome  AuditOutcome
	metadata map[string]any
}

func (a *fakeAudit) Emit(_ context.Context, action DestructiveAuditAction, outcome AuditOutcome, metadata map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := make(map[string]any, len(metadata))
	for k, v := range metadata {
		cp[k] = v
	}
	a.events = append(a.events, auditEvent{action: action, outcome: outcome, metadata: cp})
}

func (a *fakeAudit) actions() []DestructiveAuditAction {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]DestructiveAuditAction, len(a.events))
	for i, e := range a.events {
		out[i] = e.action
	}
	return out
}

// silentLogger satisfies FrequencyCorrectionLogger without printing.
type silentLogger struct{}

func (silentLogger) Info(string, ...any)  {}
func (silentLogger) Warn(string, ...any)  {}
func (silentLogger) Error(string, ...any) {}

// helper: build a registered action + emit-capture rig.
func newFreqRig(t *testing.T) (*frequencyCorrectionAction, *fakeWriter, *fakeAudit) {
	t.Helper()
	w := &fakeWriter{}
	a := &fakeAudit{}
	act := &frequencyCorrectionAction{writer: w, audit: a, log: silentLogger{}}
	return act, w, a
}

// stash the typed payload so frequencyCorrectionRequestFromGeneric
// can pick it up. Always paired with ClearFrequencyCorrectionPayload
// in the test cleanup so concurrent tests don't bleed state.
func stashPayload(t *testing.T, correlationID string, payload FrequencyCorrectionRequest) {
	t.Helper()
	SetFrequencyCorrectionPayload(correlationID, &payload)
	t.Cleanup(func() { ClearFrequencyCorrectionPayload(correlationID) })
}

// genericReq builds the generic Request the runner would dispatch.
func genericReq(correlationID string) Request {
	return Request{
		Kind:          KindFrequencyCorrection,
		DeviceID:      "device-1",
		CorrelationID: correlationID,
		DryRun:        false,
		Confirm:       true,
		Actor:         "alice",
		Reason:        "phase 10e smoke",
	}
}

// =============================================================================
// Senaryo 5 (happy path): snapshot → write → verify match → succeeded.
// =============================================================================
func TestPhase10E_HappyPath_Verified(t *testing.T) {
	corr := "corr-happy"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID: "device-1", Interface: "wlan1",
		TargetFrequencyMHz: 5180, CorrelationID: corr, RunID: "run-1",
		Actor: "alice", Intent: "phase 10e happy",
	})
	act, w, audit := newFreqRig(t)
	w.snapshotSeq = []snapshotResult{
		{mhz: 5160}, // pre-write snapshot
		{mhz: 5180}, // post-write verify
	}
	w.setSeq = []error{nil} // single write succeeds

	res, err := act.Execute(context.Background(), genericReq(corr))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !res.Success {
		t.Errorf("Result.Success = false, want true")
	}
	if res.ErrorCode != "" {
		t.Errorf("ErrorCode = %q, want empty", res.ErrorCode)
	}

	wantOrder := []DestructiveAuditAction{
		AuditActionExecuteStarted,
		AuditActionExecuteWriteSucceeded,
		AuditActionExecuteVerified,
	}
	gotOrder := audit.actions()
	if !equalActions(gotOrder, wantOrder) {
		t.Errorf("audit order:\n got  %v\n want %v", gotOrder, wantOrder)
	}
	if len(w.sets) != 1 {
		t.Errorf("device sets = %d, want 1 (just the target write)", len(w.sets))
	}
	if v := res.Result["snapshot_freq_mhz"]; v != 5160 {
		t.Errorf("snapshot recorded = %v, want 5160", v)
	}
	if v := res.Result["verified_freq_mhz"]; v != 5180 {
		t.Errorf("verified recorded = %v, want 5180", v)
	}
}

// =============================================================================
// Senaryo 9: snapshot read fails before any write.
// =============================================================================
func TestPhase10E_SnapshotFails_NoWrite(t *testing.T) {
	corr := "corr-snap-fail"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID: "device-1", Interface: "wlan1",
		TargetFrequencyMHz: 5180, CorrelationID: corr, RunID: "run-snap",
	})
	act, w, audit := newFreqRig(t)
	w.snapshotSeq = []snapshotResult{
		{err: errors.New("ssh: dial timeout")},
	}

	res, err := act.Execute(context.Background(), genericReq(corr))
	if !errors.Is(err, ErrFrequencyDeviceUnreachable) {
		t.Errorf("err = %v, want ErrFrequencyDeviceUnreachable", err)
	}
	if res.Success {
		t.Error("Result.Success must be false")
	}
	if res.ErrorCode != "device_unreachable" {
		t.Errorf("ErrorCode = %q, want device_unreachable", res.ErrorCode)
	}
	if len(w.sets) != 0 {
		t.Errorf("device sets = %d, want 0 (no write attempted)", len(w.sets))
	}
	gotOrder := audit.actions()
	wantOrder := []DestructiveAuditAction{AuditActionExecuteWriteFailed}
	if !equalActions(gotOrder, wantOrder) {
		t.Errorf("audit order:\n got  %v\n want %v", gotOrder, wantOrder)
	}
}

// =============================================================================
// Senaryo 6: snapshot OK but the write itself is rejected by the device.
// Rollback NOT attempted.
// =============================================================================
func TestPhase10E_WriteFails_NoRollback(t *testing.T) {
	corr := "corr-write-fail"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID: "device-1", Interface: "wlan1",
		TargetFrequencyMHz: 5180, CorrelationID: corr, RunID: "run-wf",
	})
	act, w, audit := newFreqRig(t)
	w.snapshotSeq = []snapshotResult{{mhz: 5160}}
	w.setSeq = []error{errors.New("device rejected: input does not match")}

	res, err := act.Execute(context.Background(), genericReq(corr))
	if !errors.Is(err, ErrFrequencyWriteFailed) {
		t.Errorf("err = %v, want ErrFrequencyWriteFailed", err)
	}
	if res.Success {
		t.Error("Result.Success must be false")
	}
	if res.ErrorCode != "write_failed" {
		t.Errorf("ErrorCode = %q, want write_failed", res.ErrorCode)
	}
	if len(w.sets) != 1 {
		t.Errorf("device sets = %d, want 1 (the failing target write only — no rollback)", len(w.sets))
	}
	wantOrder := []DestructiveAuditAction{
		AuditActionExecuteStarted,
		AuditActionExecuteWriteFailed,
	}
	gotOrder := audit.actions()
	if !equalActions(gotOrder, wantOrder) {
		t.Errorf("audit order:\n got  %v\n want %v", gotOrder, wantOrder)
	}
}

// =============================================================================
// Senaryo 7: write OK + verify mismatch → rollback succeeds.
// Terminal status: failed/error_code=verification_failed_rollback_recovered.
// =============================================================================
func TestPhase10E_VerifyFails_RollbackRecovered(t *testing.T) {
	corr := "corr-verify-fail"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID: "device-1", Interface: "wlan1",
		TargetFrequencyMHz: 5180, CorrelationID: corr, RunID: "run-vf",
	})
	act, w, audit := newFreqRig(t)
	w.snapshotSeq = []snapshotResult{
		{mhz: 5160}, // pre-write
		{mhz: 5200}, // post-write verify (device snapped to nearest channel)
		{mhz: 5160}, // post-rollback verify (snapshot restored)
	}
	w.setSeq = []error{nil, nil} // target write OK + rollback write OK

	res, err := act.Execute(context.Background(), genericReq(corr))
	if !errors.Is(err, ErrFrequencyVerificationFailedRollbackRecovered) {
		t.Errorf("err = %v, want ErrFrequencyVerificationFailedRollbackRecovered", err)
	}
	if res.Success {
		t.Error("Result.Success must be false (verification failed, even though rollback recovered)")
	}
	if res.ErrorCode != "verification_failed_rollback_recovered" {
		t.Errorf("ErrorCode = %q, want verification_failed_rollback_recovered", res.ErrorCode)
	}
	if len(w.sets) != 2 {
		t.Errorf("device sets = %d, want 2 (target write + rollback write)", len(w.sets))
	}
	if w.sets[1].mhz != 5160 {
		t.Errorf("rollback write mhz = %d, want snapshot value 5160", w.sets[1].mhz)
	}
	wantOrder := []DestructiveAuditAction{
		AuditActionExecuteStarted,
		AuditActionExecuteWriteSucceeded,
		AuditActionExecuteVerificationFailed,
		AuditActionExecuteRollbackStarted,
		AuditActionExecuteRollbackSucceeded,
	}
	gotOrder := audit.actions()
	if !equalActions(gotOrder, wantOrder) {
		t.Errorf("audit order:\n got  %v\n want %v", gotOrder, wantOrder)
	}
}

// =============================================================================
// Senaryo 8a: rollback write itself fails (device unreachable mid-run).
// =============================================================================
func TestPhase10E_VerifyFails_RollbackFails_Write(t *testing.T) {
	corr := "corr-rb-fail-w"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID: "device-1", Interface: "wlan1",
		TargetFrequencyMHz: 5180, CorrelationID: corr, RunID: "run-rb-w",
	})
	act, w, audit := newFreqRig(t)
	w.snapshotSeq = []snapshotResult{
		{mhz: 5160}, // pre-write
		{mhz: 5200}, // verify mismatch
	}
	w.setSeq = []error{
		nil,                                // target write OK
		errors.New("ssh: connection lost"), // rollback write FAILS
	}

	res, err := act.Execute(context.Background(), genericReq(corr))
	if !errors.Is(err, ErrFrequencyRollbackFailed) {
		t.Errorf("err = %v, want ErrFrequencyRollbackFailed", err)
	}
	if res.ErrorCode != "rollback_failed" {
		t.Errorf("ErrorCode = %q, want rollback_failed", res.ErrorCode)
	}
	wantOrder := []DestructiveAuditAction{
		AuditActionExecuteStarted,
		AuditActionExecuteWriteSucceeded,
		AuditActionExecuteVerificationFailed,
		AuditActionExecuteRollbackStarted,
		AuditActionExecuteRollbackFailed,
	}
	gotOrder := audit.actions()
	if !equalActions(gotOrder, wantOrder) {
		t.Errorf("audit order:\n got  %v\n want %v", gotOrder, wantOrder)
	}
}

// =============================================================================
// Senaryo 8b: rollback write succeeds but the re-verify does not match.
// =============================================================================
func TestPhase10E_VerifyFails_RollbackVerifyFails(t *testing.T) {
	corr := "corr-rb-fail-v"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID: "device-1", Interface: "wlan1",
		TargetFrequencyMHz: 5180, CorrelationID: corr, RunID: "run-rb-v",
	})
	act, w, audit := newFreqRig(t)
	w.snapshotSeq = []snapshotResult{
		{mhz: 5160}, // pre-write
		{mhz: 5200}, // verify mismatch
		{mhz: 5210}, // rollback re-verify also wrong (device flapped)
	}
	w.setSeq = []error{nil, nil} // both writes accepted

	res, err := act.Execute(context.Background(), genericReq(corr))
	if !errors.Is(err, ErrFrequencyRollbackFailed) {
		t.Errorf("err = %v, want ErrFrequencyRollbackFailed", err)
	}
	if res.ErrorCode != "rollback_failed" {
		t.Errorf("ErrorCode = %q, want rollback_failed", res.ErrorCode)
	}
	wantOrder := []DestructiveAuditAction{
		AuditActionExecuteStarted,
		AuditActionExecuteWriteSucceeded,
		AuditActionExecuteVerificationFailed,
		AuditActionExecuteRollbackStarted,
		AuditActionExecuteRollbackFailed,
	}
	gotOrder := audit.actions()
	if !equalActions(gotOrder, wantOrder) {
		t.Errorf("audit order:\n got  %v\n want %v", gotOrder, wantOrder)
	}
}

// =============================================================================
// Defensive: the side-channel payload is missing entirely. The action
// MUST refuse the run and surface request_invalid; this is the only
// branch the handler's invariant-violation guard catches.
// =============================================================================
func TestPhase10E_TypedPayloadMissing_Refuses(t *testing.T) {
	act, w, audit := newFreqRig(t)
	res, err := act.Execute(context.Background(), genericReq("corr-no-payload"))
	if err != nil {
		// finish() returns nil for the request_invalid code on
		// purpose; the handler reads ErrorCode, not the error.
		t.Errorf("err = %v, want nil (handler reads ErrorCode)", err)
	}
	if res.Success {
		t.Error("Result.Success must be false")
	}
	if res.ErrorCode != "request_invalid" {
		t.Errorf("ErrorCode = %q, want request_invalid", res.ErrorCode)
	}
	if len(w.sets) != 0 {
		t.Errorf("no device write should be issued; got %d", len(w.sets))
	}
	if len(audit.events) != 0 {
		t.Errorf("no audit event should be emitted; got %v", audit.actions())
	}
}

// Defensive: payload present but interface is empty.
func TestPhase10E_TypedPayloadIncomplete_Refuses(t *testing.T) {
	corr := "corr-incomplete"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID: "device-1",
		// Interface intentionally empty.
		TargetFrequencyMHz: 5180,
		CorrelationID:      corr,
	})
	act, _, _ := newFreqRig(t)
	res, _ := act.Execute(context.Background(), genericReq(corr))
	if res.ErrorCode != "request_invalid" {
		t.Errorf("ErrorCode = %q, want request_invalid", res.ErrorCode)
	}
}

func TestPhase10E_TypedPayloadInvalidFreq_Refuses(t *testing.T) {
	corr := "corr-bad-freq"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID:           "device-1",
		Interface:          "wlan1",
		TargetFrequencyMHz: 0,
		CorrelationID:      corr,
	})
	act, _, _ := newFreqRig(t)
	res, _ := act.Execute(context.Background(), genericReq(corr))
	if res.ErrorCode != "request_invalid" {
		t.Errorf("ErrorCode = %q, want request_invalid", res.ErrorCode)
	}
}

// =============================================================================
// Snapshot returns a non-positive value — treat as snapshot_unreadable
// to avoid using a bogus rollback anchor.
// =============================================================================
func TestPhase10E_SnapshotZero_Unreadable(t *testing.T) {
	corr := "corr-snap-zero"
	stashPayload(t, corr, FrequencyCorrectionRequest{
		DeviceID: "device-1", Interface: "wlan1",
		TargetFrequencyMHz: 5180, CorrelationID: corr,
	})
	act, w, audit := newFreqRig(t)
	w.snapshotSeq = []snapshotResult{{mhz: 0}}

	res, err := act.Execute(context.Background(), genericReq(corr))
	if !errors.Is(err, ErrFrequencyUnreadable) {
		t.Errorf("err = %v, want ErrFrequencyUnreadable", err)
	}
	if res.ErrorCode != "snapshot_unreadable" {
		t.Errorf("ErrorCode = %q, want snapshot_unreadable", res.ErrorCode)
	}
	if len(w.sets) != 0 {
		t.Error("no write should be attempted on unreadable snapshot")
	}
	if len(audit.events) != 1 || audit.events[0].action != AuditActionExecuteWriteFailed {
		t.Errorf("expected single execute_write_failed event, got %v", audit.actions())
	}
}

// =============================================================================
// Registration: RegisterFrequencyCorrection replaces the registry stub.
// =============================================================================
func TestPhase10E_Register_ReplacesStub(t *testing.T) {
	r := NewRegistry()
	// Initial stub returns ErrActionNotImplemented.
	stub := r.Get(KindFrequencyCorrection)
	if stub == nil {
		t.Fatal("registry should populate a stub initially")
	}
	res, err := stub.Execute(context.Background(), Request{Kind: KindFrequencyCorrection})
	if !errors.Is(err, ErrActionNotImplemented) {
		t.Errorf("default stub.Execute = %v, want ErrActionNotImplemented", err)
	}
	_ = res

	// After RegisterFrequencyCorrection the registry returns the real action.
	w := &fakeWriter{}
	a := &fakeAudit{}
	RegisterFrequencyCorrection(r, w, a, silentLogger{})
	got := r.Get(KindFrequencyCorrection)
	if got == nil {
		t.Fatal("registry has no entry after register")
	}
	if _, isStub := got.(stubAction); isStub {
		t.Error("registry still returns stubAction after RegisterFrequencyCorrection")
	}
	if got.Kind() != KindFrequencyCorrection {
		t.Errorf("registered Kind = %q, want %q", got.Kind(), KindFrequencyCorrection)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func equalActions(got, want []DestructiveAuditAction) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
