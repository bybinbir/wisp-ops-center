package workorders

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound, kayıt bulunamadığında döner.
var ErrNotFound = errors.New("workorders: not found")

// ErrAlreadyPromoted, aday zaten gerçek iş emrine dönüştürülmüşse döner.
var ErrAlreadyPromoted = errors.New("workorders: candidate already promoted")

// ErrCandidateNotPromotable, aday open olmadığında döner (dismissed/cancelled).
var ErrCandidateNotPromotable = errors.New("workorders: candidate not promotable")

// Repository, work_orders + work_order_events + work_order_candidates'ı
// promote akışıyla birlikte yönetir.
type Repository struct {
	P *pgxpool.Pool
}

// NewRepository, pgxpool ile yeni bir repo döndürür.
func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{P: p} }

// WorkOrder, API ve DB satırı.
type WorkOrder struct {
	ID                string     `json:"id"`
	CustomerID        *string    `json:"customer_id,omitempty"`
	APDeviceID        *string    `json:"ap_device_id,omitempty"`
	TowerID           *string    `json:"tower_id,omitempty"`
	SourceCandidateID *string    `json:"source_candidate_id,omitempty"`
	SourceScoreID     *string    `json:"source_score_id,omitempty"`
	Diagnosis         string     `json:"diagnosis"`
	RecommendedAction string     `json:"recommended_action"`
	Severity          string     `json:"severity"`
	Title             string     `json:"title"`
	Description       string     `json:"description"`
	Status            string     `json:"status"`
	Priority          string     `json:"priority"`
	AssignedTo        *string    `json:"assigned_to,omitempty"`
	ETAAt             *time.Time `json:"eta_at,omitempty"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	ResolutionNote    *string    `json:"resolution_note,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Event, work_order_events satırı.
type Event struct {
	ID          string    `json:"id"`
	WorkOrderID string    `json:"work_order_id"`
	EventType   string    `json:"event_type"`
	OldValue    *string   `json:"old_value,omitempty"`
	NewValue    *string   `json:"new_value,omitempty"`
	Note        *string   `json:"note,omitempty"`
	Actor       string    `json:"actor"`
	CreatedAt   time.Time `json:"created_at"`
}

const woCols = `id::text, customer_id::text, ap_device_id::text, tower_id::text,
       source_candidate_id::text, source_score_id::text,
       diagnosis, recommended_action, severity,
       title, COALESCE(description,''),
       status, priority, assigned_to,
       eta_at, resolved_at, resolution_note,
       created_at, updated_at`

func scanWO(row pgx.Row) (*WorkOrder, error) {
	var w WorkOrder
	if err := row.Scan(
		&w.ID, &w.CustomerID, &w.APDeviceID, &w.TowerID,
		&w.SourceCandidateID, &w.SourceScoreID,
		&w.Diagnosis, &w.RecommendedAction, &w.Severity,
		&w.Title, &w.Description,
		&w.Status, &w.Priority, &w.AssignedTo,
		&w.ETAAt, &w.ResolvedAt, &w.ResolutionNote,
		&w.CreatedAt, &w.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &w, nil
}

// PromoteInput, aday → iş emri promosyon girdisi.
type PromoteInput struct {
	CandidateID string
	Title       string
	Description string
	Priority    Priority
	AssignedTo  *string
	ETAAt       *time.Time
	Actor       string
}

// PromoteOutcome, promosyon sonucu.
type PromoteOutcome struct {
	WorkOrder    *WorkOrder
	Duplicate    bool
	DuplicateRef string
}

// PromoteCandidate, work_order_candidates satırını gerçek iş emrine dönüştürür.
//
// Kurallar:
//   - Aday yoksa ErrNotFound.
//   - Aday status='promoted' ve promoted_work_order_id varsa Duplicate=true,
//     mevcut iş emri döner. Yeni satır oluşturulmaz.
//   - Aday status 'dismissed' veya 'cancelled' ise ErrCandidateNotPromotable.
//   - Promosyon tek bir transaction içinde:
//   - work_orders satırı insert
//   - work_order_events 'created' kaydı
//   - work_order_candidates.status='promoted', promoted_work_order_id=...
func (r *Repository) PromoteCandidate(ctx context.Context, in PromoteInput) (PromoteOutcome, error) {
	if in.CandidateID == "" {
		return PromoteOutcome{}, errors.New("workorders: candidate_id required")
	}
	if !IsValidPriority(string(in.Priority)) {
		in.Priority = PriorityMedium
	}
	if in.Actor == "" {
		in.Actor = "system"
	}

	tx, err := r.P.Begin(ctx)
	if err != nil {
		return PromoteOutcome{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Adayı kilitle.
	var (
		candStatus     string
		candCustomerID *string
		candAPDeviceID *string
		candTowerID    *string
		candScoreID    *string
		candDiagnosis  string
		candAction     string
		candSeverity   string
		promotedWOID   *string
	)
	err = tx.QueryRow(ctx, `
SELECT status, customer_id::text, ap_device_id::text, tower_id::text,
       source_score_id::text, diagnosis, recommended_action, severity,
       promoted_work_order_id::text
  FROM work_order_candidates
 WHERE id = $1
 FOR UPDATE`, in.CandidateID).Scan(
		&candStatus, &candCustomerID, &candAPDeviceID, &candTowerID,
		&candScoreID, &candDiagnosis, &candAction, &candSeverity,
		&promotedWOID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PromoteOutcome{}, ErrNotFound
	}
	if err != nil {
		return PromoteOutcome{}, err
	}

	// Zaten promote edilmişse mevcut iş emrini döndür.
	if candStatus == "promoted" && promotedWOID != nil && *promotedWOID != "" {
		row := tx.QueryRow(ctx, `SELECT `+woCols+` FROM work_orders WHERE id = $1`, *promotedWOID)
		wo, err := scanWO(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return PromoteOutcome{}, ErrAlreadyPromoted
			}
			return PromoteOutcome{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return PromoteOutcome{}, err
		}
		return PromoteOutcome{WorkOrder: wo, Duplicate: true, DuplicateRef: wo.ID}, nil
	}

	if candStatus != "open" {
		return PromoteOutcome{}, fmt.Errorf("%w: status=%s", ErrCandidateNotPromotable, candStatus)
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = humanTitle(candDiagnosis, candCustomerID)
	}

	// Aday yeni iş emri olarak insert.
	row := tx.QueryRow(ctx, `
INSERT INTO work_orders(
  customer_id, ap_device_id, tower_id, source_candidate_id, source_score_id,
  diagnosis, recommended_action, severity, title, description,
  status, priority, assigned_to, eta_at
) VALUES (
  NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid,
  $4::uuid, NULLIF($5,'')::uuid,
  $6, $7, $8, $9, $10,
  'open', $11, $12, $13
)
RETURNING `+woCols,
		strOrEmpty(candCustomerID), strOrEmpty(candAPDeviceID), strOrEmpty(candTowerID),
		in.CandidateID, strOrEmpty(candScoreID),
		candDiagnosis, candAction, candSeverity, title, in.Description,
		string(in.Priority), in.AssignedTo, in.ETAAt,
	)
	wo, err := scanWO(row)
	if err != nil {
		return PromoteOutcome{}, err
	}

	// İlk event: created.
	if _, err := tx.Exec(ctx, `
INSERT INTO work_order_events(work_order_id, event_type, new_value, note, actor)
VALUES ($1, $2, $3, $4, $5)`,
		wo.ID, string(EventCreated), "open",
		"Aday "+in.CandidateID+" iş emrine promote edildi", in.Actor); err != nil {
		return PromoteOutcome{}, err
	}
	if in.AssignedTo != nil && *in.AssignedTo != "" {
		if _, err := tx.Exec(ctx, `
INSERT INTO work_order_events(work_order_id, event_type, new_value, actor)
VALUES ($1, $2, $3, $4)`, wo.ID, string(EventAssigned), *in.AssignedTo, in.Actor); err != nil {
			return PromoteOutcome{}, err
		}
	}

	// Adayı promoted işaretle.
	if _, err := tx.Exec(ctx, `
UPDATE work_order_candidates
   SET status = 'promoted',
       promoted_work_order_id = $2::uuid,
       updated_at = now()
 WHERE id = $1`, in.CandidateID, wo.ID); err != nil {
		return PromoteOutcome{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return PromoteOutcome{}, err
	}
	return PromoteOutcome{WorkOrder: wo, Duplicate: false}, nil
}

func humanTitle(diagnosis string, customerID *string) string {
	cust := ""
	if customerID != nil && *customerID != "" {
		cust = " · müşteri " + (*customerID)[:8]
	}
	return strings.TrimSpace("İş Emri: " + diagnosis + cust)
}

// ListFilter, /api/v1/work-orders filtreleri.
type ListFilter struct {
	Status     string
	Priority   string
	Severity   string
	TowerID    string
	APDeviceID string
	CustomerID string
	AssignedTo string
	DateFrom   *time.Time
	DateTo     *time.Time
	Limit      int
	Offset     int
}

// List, work_orders satırlarını filtreyle döner.
func (r *Repository) List(ctx context.Context, f ListFilter) ([]WorkOrder, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	args := []any{}
	conds := []string{"1=1"}
	add := func(cond string, a ...any) {
		args = append(args, a...)
		conds = append(conds, cond)
	}
	if f.Status != "" {
		add("status = $"+strconv.Itoa(len(args)+1), f.Status)
	}
	if f.Priority != "" {
		add("priority = $"+strconv.Itoa(len(args)+1), f.Priority)
	}
	if f.Severity != "" {
		add("severity = $"+strconv.Itoa(len(args)+1), f.Severity)
	}
	if f.TowerID != "" {
		add("tower_id = $"+strconv.Itoa(len(args)+1)+"::uuid", f.TowerID)
	}
	if f.APDeviceID != "" {
		add("ap_device_id = $"+strconv.Itoa(len(args)+1)+"::uuid", f.APDeviceID)
	}
	if f.CustomerID != "" {
		add("customer_id = $"+strconv.Itoa(len(args)+1)+"::uuid", f.CustomerID)
	}
	if f.AssignedTo != "" {
		add("assigned_to = $"+strconv.Itoa(len(args)+1), f.AssignedTo)
	}
	if f.DateFrom != nil {
		add("created_at >= $"+strconv.Itoa(len(args)+1), *f.DateFrom)
	}
	if f.DateTo != nil {
		add("created_at <= $"+strconv.Itoa(len(args)+1), *f.DateTo)
	}
	where := strings.Join(conds, " AND ")

	// Toplam sayım — UI sayfa kontrolü için.
	var total int
	if err := r.P.QueryRow(ctx, `SELECT COUNT(*) FROM work_orders WHERE `+where, args...).
		Scan(&total); err != nil {
		return nil, 0, err
	}

	limitArgPos := strconv.Itoa(len(args) + 1)
	offsetArgPos := strconv.Itoa(len(args) + 2)
	args = append(args, f.Limit, f.Offset)

	q := `SELECT ` + woCols + `
            FROM work_orders
           WHERE ` + where + `
           ORDER BY
             CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1
                           WHEN 'medium' THEN 2 ELSE 3 END,
             CASE status   WHEN 'open' THEN 0 WHEN 'assigned' THEN 1
                           WHEN 'in_progress' THEN 2 WHEN 'cancelled' THEN 3
                           WHEN 'resolved' THEN 4 ELSE 5 END,
             created_at DESC
           LIMIT $` + limitArgPos + ` OFFSET $` + offsetArgPos
	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []WorkOrder{}
	for rows.Next() {
		w, err := scanWO(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *w)
	}
	return out, total, rows.Err()
}

// Get, tek bir iş emrini döner.
func (r *Repository) Get(ctx context.Context, id string) (*WorkOrder, error) {
	row := r.P.QueryRow(ctx, `SELECT `+woCols+` FROM work_orders WHERE id = $1`, id)
	wo, err := scanWO(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return wo, err
}

// PatchInput, /api/v1/work-orders/{id} PATCH girdisi. Yalnız non-nil alanlar
// güncellenir. Status değişikliği için Status alanı set edilmeli; geçersiz
// geçişler ErrInvalidTransition ile reddedilir.
type PatchInput struct {
	Status      *string
	Priority    *string
	AssignedTo  *string
	Title       *string
	Description *string
	ETAAt       *time.Time
	ClearETA    bool
	Note        *string
	Actor       string
}

// Patch, work order satırını günceller ve değişiklik başına bir event yazar.
func (r *Repository) Patch(ctx context.Context, id string, in PatchInput) (*WorkOrder, error) {
	if in.Actor == "" {
		in.Actor = "system"
	}
	tx, err := r.P.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := func() (*WorkOrder, error) {
		row := tx.QueryRow(ctx, `SELECT `+woCols+` FROM work_orders WHERE id = $1 FOR UPDATE`, id)
		return scanWO(row)
	}()
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	sets := []string{"updated_at = now()"}
	args := []any{}

	events := []Event{}
	addEvt := func(et EventType, oldV, newV, note *string) {
		ev := Event{EventType: string(et), Actor: in.Actor}
		ev.OldValue = oldV
		ev.NewValue = newV
		ev.Note = note
		events = append(events, ev)
	}

	if in.Status != nil && *in.Status != "" {
		if !IsValidStatus(*in.Status) {
			return nil, fmt.Errorf("invalid status: %s", *in.Status)
		}
		from := Status(current.Status)
		to := Status(*in.Status)
		if from != to {
			if !CanTransition(from, to) {
				return nil, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
			}
			args = append(args, *in.Status)
			sets = append(sets, "status = $"+strconv.Itoa(len(args)))
			oldV := current.Status
			addEvt(EventStatusChanged, &oldV, in.Status, in.Note)
			if to == StatusResolved {
				sets = append(sets, "resolved_at = now()")
			}
		}
	}
	if in.Priority != nil && *in.Priority != "" {
		if !IsValidPriority(*in.Priority) {
			return nil, fmt.Errorf("invalid priority: %s", *in.Priority)
		}
		if *in.Priority != current.Priority {
			args = append(args, *in.Priority)
			sets = append(sets, "priority = $"+strconv.Itoa(len(args)))
			oldV := current.Priority
			addEvt(EventPriorityChanged, &oldV, in.Priority, nil)
		}
	}
	if in.AssignedTo != nil {
		newVal := *in.AssignedTo
		oldVal := ""
		if current.AssignedTo != nil {
			oldVal = *current.AssignedTo
		}
		if newVal != oldVal {
			if newVal == "" {
				sets = append(sets, "assigned_to = NULL")
				addEvt(EventUnassigned, &oldVal, nil, nil)
			} else {
				args = append(args, newVal)
				sets = append(sets, "assigned_to = $"+strconv.Itoa(len(args)))
				addEvt(EventAssigned, nil, &newVal, nil)
			}
		}
	}
	if in.Title != nil && *in.Title != "" && *in.Title != current.Title {
		args = append(args, *in.Title)
		sets = append(sets, "title = $"+strconv.Itoa(len(args)))
	}
	if in.Description != nil {
		args = append(args, *in.Description)
		sets = append(sets, "description = $"+strconv.Itoa(len(args)))
	}
	if in.ClearETA {
		sets = append(sets, "eta_at = NULL")
		addEvt(EventETAUpdated, nil, nil, nil)
	} else if in.ETAAt != nil {
		args = append(args, *in.ETAAt)
		sets = append(sets, "eta_at = $"+strconv.Itoa(len(args)))
		newV := in.ETAAt.UTC().Format(time.RFC3339)
		addEvt(EventETAUpdated, nil, &newV, nil)
	}
	if in.Note != nil && *in.Note != "" {
		// Sadece note geldiyse ayrı bir 'note_added' event'i yaz.
		// Status veya assign üzerinde inline kullanılan note'lar zaten
		// üst event'e eklendi.
		hasInlined := false
		for _, e := range events {
			if e.Note != nil {
				hasInlined = true
				break
			}
		}
		if !hasInlined {
			n := *in.Note
			addEvt(EventNoteAdded, nil, nil, &n)
		}
	}

	if len(sets) > 1 || len(events) > 0 {
		args = append(args, id)
		_, err = tx.Exec(ctx,
			`UPDATE work_orders SET `+strings.Join(sets, ", ")+
				` WHERE id = $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			return nil, err
		}
	}
	for _, ev := range events {
		if _, err := tx.Exec(ctx, `
INSERT INTO work_order_events(work_order_id, event_type, old_value, new_value, note, actor)
VALUES ($1,$2,$3,$4,$5,$6)`,
			id, ev.EventType, ev.OldValue, ev.NewValue, ev.Note, in.Actor); err != nil {
			return nil, err
		}
	}

	row := tx.QueryRow(ctx, `SELECT `+woCols+` FROM work_orders WHERE id = $1`, id)
	wo, err := scanWO(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return wo, nil
}

// Resolve, in_progress / open / assigned → resolved kısayolu.
func (r *Repository) Resolve(ctx context.Context, id string, note, actor string) (*WorkOrder, error) {
	st := string(StatusResolved)
	tx, err := r.P.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current WorkOrder
	row := tx.QueryRow(ctx, `SELECT `+woCols+` FROM work_orders WHERE id = $1 FOR UPDATE`, id)
	c, err := scanWO(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	current = *c
	if !CanTransition(Status(current.Status), StatusResolved) {
		return nil, fmt.Errorf("%w: %s → resolved", ErrInvalidTransition, current.Status)
	}
	args := []any{st, id}
	q := `UPDATE work_orders SET status=$1, resolved_at=now(), updated_at=now()`
	if note != "" {
		args = append([]any{note}, args...)
		q = `UPDATE work_orders SET resolution_note=$1, status=$2, resolved_at=now(), updated_at=now() WHERE id=$3`
	} else {
		q += ` WHERE id=$2`
	}
	if _, err := tx.Exec(ctx, q, args...); err != nil {
		return nil, err
	}
	oldV := current.Status
	if _, err := tx.Exec(ctx, `
INSERT INTO work_order_events(work_order_id, event_type, old_value, new_value, note, actor)
VALUES ($1,$2,$3,$4,$5,$6)`,
		id, string(EventResolved), oldV, st, nullable(note), actor); err != nil {
		return nil, err
	}
	row = tx.QueryRow(ctx, `SELECT `+woCols+` FROM work_orders WHERE id = $1`, id)
	wo, err := scanWO(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return wo, nil
}

// Cancel, work order'ı iptal eder.
func (r *Repository) Cancel(ctx context.Context, id, note, actor string) (*WorkOrder, error) {
	tx, err := r.P.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `SELECT `+woCols+` FROM work_orders WHERE id = $1 FOR UPDATE`, id)
	c, err := scanWO(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !CanTransition(Status(c.Status), StatusCancelled) {
		return nil, fmt.Errorf("%w: %s → cancelled", ErrInvalidTransition, c.Status)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE work_orders SET status='cancelled', resolution_note=$2, updated_at=now() WHERE id=$1`,
		id, nullable(note)); err != nil {
		return nil, err
	}
	oldV := c.Status
	if _, err := tx.Exec(ctx, `
INSERT INTO work_order_events(work_order_id, event_type, old_value, new_value, note, actor)
VALUES ($1,$2,$3,'cancelled',$4,$5)`,
		id, string(EventCancelled), oldV, nullable(note), actor); err != nil {
		return nil, err
	}
	row = tx.QueryRow(ctx, `SELECT `+woCols+` FROM work_orders WHERE id = $1`, id)
	wo, err := scanWO(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return wo, nil
}

// Assign, work_orders.assigned_to alanını set eder ve event yazar.
// AssignedTo "" gönderilirse atamayı kaldırır.
func (r *Repository) Assign(ctx context.Context, id, assignee, note, actor string) (*WorkOrder, error) {
	in := PatchInput{
		AssignedTo: &assignee,
		Note:       nullable(note),
		Actor:      actor,
	}
	if assignee != "" {
		// open → assigned otomatik geçiş yapma; operatör isterse
		// PATCH ile status'u ayrıca değiştirir. Bu sayede in_progress
		// üzerindeki bir iş emri assignee değiştirildiğinde status
		// resetlenmez.
	}
	return r.Patch(ctx, id, in)
}

// AppendEvent, work_order_events satırı ekler (manuel "note" akışı için).
func (r *Repository) AppendEvent(ctx context.Context, id, eventType, note, actor string) (*Event, error) {
	if eventType == "" {
		eventType = string(EventNoteAdded)
	}
	if actor == "" {
		actor = "system"
	}
	row := r.P.QueryRow(ctx, `
INSERT INTO work_order_events(work_order_id, event_type, note, actor)
VALUES ($1,$2,$3,$4)
RETURNING id::text, work_order_id::text, event_type, old_value, new_value, note, actor, created_at`,
		id, eventType, nullable(note), actor)
	var ev Event
	if err := row.Scan(&ev.ID, &ev.WorkOrderID, &ev.EventType,
		&ev.OldValue, &ev.NewValue, &ev.Note, &ev.Actor, &ev.CreatedAt); err != nil {
		return nil, err
	}
	return &ev, nil
}

// ListEvents, work_order_events satırlarını döner (en eski → yeni).
func (r *Repository) ListEvents(ctx context.Context, workOrderID string) ([]Event, error) {
	rows, err := r.P.Query(ctx, `
SELECT id::text, work_order_id::text, event_type, old_value, new_value, note, actor, created_at
  FROM work_order_events
 WHERE work_order_id = $1
 ORDER BY created_at ASC`, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.WorkOrderID, &ev.EventType,
			&ev.OldValue, &ev.NewValue, &ev.Note, &ev.Actor, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// Counts, dashboard için açık/urgent/eta-geçen/bugün-oluşturulan sayıları.
type Counts struct {
	Open         int        `json:"open"`
	Assigned     int        `json:"assigned"`
	InProgress   int        `json:"in_progress"`
	Resolved7d   int        `json:"resolved_7d"`
	Cancelled7d  int        `json:"cancelled_7d"`
	UrgentOrHigh int        `json:"urgent_or_high"`
	OverdueETA   int        `json:"overdue_eta"`
	CreatedToday int        `json:"created_today"`
	NowAt        time.Time  `json:"now_at"`
	OldestOpenAt *time.Time `json:"oldest_open_at,omitempty"`
}

// Counts döner.
func (r *Repository) Counts(ctx context.Context) (Counts, error) {
	c := Counts{NowAt: time.Now().UTC()}
	row := r.P.QueryRow(ctx, `
SELECT
  COUNT(*) FILTER (WHERE status='open')                                 AS open,
  COUNT(*) FILTER (WHERE status='assigned')                             AS assigned,
  COUNT(*) FILTER (WHERE status='in_progress')                          AS in_progress,
  COUNT(*) FILTER (WHERE status='resolved'  AND resolved_at >= now() - interval '7 days') AS resolved_7d,
  COUNT(*) FILTER (WHERE status='cancelled' AND updated_at  >= now() - interval '7 days') AS cancelled_7d,
  COUNT(*) FILTER (WHERE priority IN ('urgent','high') AND status NOT IN ('resolved','cancelled')) AS urgent_or_high,
  COUNT(*) FILTER (WHERE eta_at IS NOT NULL AND eta_at < now()
                       AND status NOT IN ('resolved','cancelled'))      AS overdue_eta,
  COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now()))        AS created_today,
  MIN(created_at) FILTER (WHERE status NOT IN ('resolved','cancelled')) AS oldest_open_at
FROM work_orders`)
	if err := row.Scan(
		&c.Open, &c.Assigned, &c.InProgress, &c.Resolved7d, &c.Cancelled7d,
		&c.UrgentOrHigh, &c.OverdueETA, &c.CreatedToday, &c.OldestOpenAt,
	); err != nil {
		return c, err
	}
	return c, nil
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
