package scoring

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound, kayıt bulunamadığında döner.
var ErrNotFound = errors.New("scoring: not found")

// Repository, skorlama tablolarını yönetir.
type Repository struct {
	P *pgxpool.Pool
}

// NewRepository, pgxpool ile yeni bir repo döndürür.
func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{P: p} }

// ============================================================================
// Thresholds
// ============================================================================

// LoadThresholds, scoring_thresholds tablosundan tüm anahtar/değerleri okur
// ve DefaultThresholds() üstüne uygular.
func (r *Repository) LoadThresholds(ctx context.Context) (Thresholds, error) {
	thr := DefaultThresholds()
	if r == nil || r.P == nil {
		return thr, nil
	}
	rows, err := r.P.Query(ctx, `SELECT key, value FROM scoring_thresholds`)
	if err != nil {
		return thr, err
	}
	defer rows.Close()
	overrides := map[string]float64{}
	for rows.Next() {
		var k string
		var v float64
		if err := rows.Scan(&k, &v); err != nil {
			return thr, err
		}
		overrides[k] = v
	}
	if err := rows.Err(); err != nil {
		return thr, err
	}
	return thr.ApplyOverrides(overrides), nil
}

// ThresholdRow, scoring_thresholds API row'u.
type ThresholdRow struct {
	Key         string    `json:"key"`
	Value       float64   `json:"value"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   *string   `json:"updated_by,omitempty"`
}

// ListThresholds, tüm eşik satırlarını döner.
func (r *Repository) ListThresholds(ctx context.Context) ([]ThresholdRow, error) {
	rows, err := r.P.Query(ctx, `
		SELECT key, value, COALESCE(description,''), updated_at, updated_by
		FROM scoring_thresholds
		ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ThresholdRow{}
	for rows.Next() {
		var t ThresholdRow
		if err := rows.Scan(&t.Key, &t.Value, &t.Description, &t.UpdatedAt, &t.UpdatedBy); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpsertThreshold, tek bir eşiği günceller.
func (r *Repository) UpsertThreshold(ctx context.Context, key string, value float64, by string) error {
	_, err := r.P.Exec(ctx, `
		INSERT INTO scoring_thresholds (key, value, description, updated_at, updated_by)
		VALUES ($1, $2, '', now(), $3)
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = now(),
		    updated_by = EXCLUDED.updated_by`,
		key, value, by)
	return err
}

// ============================================================================
// Customer scores
// ============================================================================

// CustomerScoreRow, customer_signal_scores satırı.
type CustomerScoreRow struct {
	ID                  string             `json:"id"`
	CustomerID          string             `json:"customer_id"`
	APDeviceID          *string            `json:"ap_device_id,omitempty"`
	TowerID             *string            `json:"tower_id,omitempty"`
	Score               int                `json:"score"`
	Severity            string             `json:"severity"`
	Diagnosis           string             `json:"diagnosis"`
	RecommendedAction   string             `json:"recommended_action"`
	Reasons             []string           `json:"reasons"`
	ContributingMetrics map[string]float64 `json:"contributing_metrics"`
	RSSIdBm             *float64           `json:"rssi_dbm,omitempty"`
	SNRdB               *float64           `json:"snr_db,omitempty"`
	CCQ                 *float64           `json:"ccq,omitempty"`
	PacketLossPct       *float64           `json:"packet_loss_pct,omitempty"`
	AvgLatencyMs        *float64           `json:"avg_latency_ms,omitempty"`
	JitterMs            *float64           `json:"jitter_ms,omitempty"`
	SignalTrend7d       *float64           `json:"signal_trend_7d,omitempty"`
	IsStale             bool               `json:"is_stale"`
	CalculatedAt        time.Time          `json:"calculated_at"`
}

// SaveCustomerScore, hesaplanan skoru kalıcı tabloya yazar
// ve customers cache satırını günceller.
func (r *Repository) SaveCustomerScore(ctx context.Context, customerID string,
	apDeviceID, towerID *string, in Inputs, res Result) (string, error) {
	reasonsJSON, _ := json.Marshal(res.Reasons)
	cmJSON, _ := json.Marshal(res.ContributingMetrics)

	tx, err := r.P.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO customer_signal_scores (
		  customer_id, ap_device_id, tower_id, score, severity, diagnosis,
		  recommended_action, reasons, contributing_metrics,
		  rssi_dbm, snr_db, ccq, packet_loss_pct, avg_latency_ms, jitter_ms,
		  signal_trend_7d, is_stale, calculated_at
		) VALUES (
		  $1, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, $4, $5, $6,
		  $7, $8::jsonb, $9::jsonb,
		  $10, $11, $12, $13, $14, $15,
		  $16, $17, $18
		) RETURNING id`,
		customerID,
		strOrEmpty(apDeviceID),
		strOrEmpty(towerID),
		res.Score, string(res.Severity), string(res.Diagnosis),
		string(res.RecommendedAction), string(reasonsJSON), string(cmJSON),
		in.RSSIdBm, in.SNRdB, in.CCQ, in.PacketLossPct, in.AvgLatencyMs, in.JitterMs,
		in.SignalTrend7d, res.IsStale, res.CalculatedAt,
	).Scan(&id)
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(ctx, `
		UPDATE customers SET
		  last_signal_score     = $1,
		  last_signal_severity  = $2,
		  last_signal_diagnosis = $3,
		  last_signal_at        = $4
		WHERE id = $5`,
		res.Score, string(res.Severity), string(res.Diagnosis), res.CalculatedAt, customerID)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// LatestCustomerScore, müşterinin en son skor satırını döner.
func (r *Repository) LatestCustomerScore(ctx context.Context, customerID string) (*CustomerScoreRow, error) {
	row := r.P.QueryRow(ctx, `
		SELECT id, customer_id, ap_device_id::text, tower_id::text,
		       score, severity, diagnosis, recommended_action,
		       reasons, contributing_metrics,
		       rssi_dbm, snr_db, ccq, packet_loss_pct, avg_latency_ms, jitter_ms,
		       signal_trend_7d, is_stale, calculated_at
		FROM customer_signal_scores
		WHERE customer_id = $1
		ORDER BY calculated_at DESC LIMIT 1`, customerID)
	out, err := scanCustomerScore(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out, nil
}

// CustomerScoreHistory, müşterinin geçmiş skor satırlarını döner.
func (r *Repository) CustomerScoreHistory(ctx context.Context, customerID string, limit int) ([]CustomerScoreRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.P.Query(ctx, `
		SELECT id, customer_id, ap_device_id::text, tower_id::text,
		       score, severity, diagnosis, recommended_action,
		       reasons, contributing_metrics,
		       rssi_dbm, snr_db, ccq, packet_loss_pct, avg_latency_ms, jitter_ms,
		       signal_trend_7d, is_stale, calculated_at
		FROM customer_signal_scores
		WHERE customer_id = $1
		ORDER BY calculated_at DESC LIMIT $2`, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerScoreRow{}
	for rows.Next() {
		r2, err := scanCustomerScore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r2)
	}
	return out, rows.Err()
}

// CustomerWithIssuesFilter, /customers/with-issues filtresi.
type CustomerWithIssuesFilter struct {
	Severity   string // "critical" | "warning" | "" (boş = tüm sorunlu)
	Diagnosis  string
	TowerID    string
	APDeviceID string
	OnlyStale  bool
	Limit      int
	Offset     int
}

// CustomerWithIssueRow, problem müşteri listeleme satırı (joined view).
type CustomerWithIssueRow struct {
	CustomerID        string    `json:"customer_id"`
	CustomerName      string    `json:"customer_name"`
	APDeviceID        *string   `json:"ap_device_id,omitempty"`
	TowerID           *string   `json:"tower_id,omitempty"`
	Score             int       `json:"score"`
	Severity          string    `json:"severity"`
	Diagnosis         string    `json:"diagnosis"`
	RecommendedAction string    `json:"recommended_action"`
	IsStale           bool      `json:"is_stale"`
	CalculatedAt      time.Time `json:"calculated_at"`
}

// CustomersWithIssues, "Sorunlu Müşteriler" listesini üretir.
func (r *Repository) CustomersWithIssues(ctx context.Context, f CustomerWithIssuesFilter) ([]CustomerWithIssueRow, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	args := []any{}
	conds := []string{"css.id = (SELECT id FROM customer_signal_scores WHERE customer_id = c.id ORDER BY calculated_at DESC LIMIT 1)"}

	if f.Severity != "" {
		args = append(args, f.Severity)
		conds = append(conds, "css.severity = $"+itoa(len(args)))
	} else {
		// Default: yalnızca warning + critical
		conds = append(conds, "css.severity IN ('warning','critical')")
	}
	if f.Diagnosis != "" {
		args = append(args, f.Diagnosis)
		conds = append(conds, "css.diagnosis = $"+itoa(len(args)))
	}
	if f.TowerID != "" {
		args = append(args, f.TowerID)
		conds = append(conds, "css.tower_id = $"+itoa(len(args))+"::uuid")
	}
	if f.APDeviceID != "" {
		args = append(args, f.APDeviceID)
		conds = append(conds, "css.ap_device_id = $"+itoa(len(args))+"::uuid")
	}
	if f.OnlyStale {
		conds = append(conds, "css.is_stale = true")
	}

	q := `SELECT c.id, c.full_name, css.ap_device_id::text, css.tower_id::text,
	             css.score, css.severity, css.diagnosis, css.recommended_action,
	             css.is_stale, css.calculated_at
	      FROM customers c
	      JOIN customer_signal_scores css ON css.customer_id = c.id
	      WHERE ` + joinAnd(conds) + `
	      ORDER BY
	        CASE css.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
	        css.score ASC,
	        css.calculated_at DESC
	      LIMIT $` + itoa(len(args)+1) + ` OFFSET $` + itoa(len(args)+2)
	args = append(args, f.Limit, f.Offset)

	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerWithIssueRow{}
	for rows.Next() {
		var w CustomerWithIssueRow
		if err := rows.Scan(&w.CustomerID, &w.CustomerName, &w.APDeviceID, &w.TowerID,
			&w.Score, &w.Severity, &w.Diagnosis, &w.RecommendedAction,
			&w.IsStale, &w.CalculatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ============================================================================
// AP / Tower scores
// ============================================================================

// SaveAPScore, ap_health_scores tablosuna yazar.
func (r *Repository) SaveAPScore(ctx context.Context, in APInputs, res APResult) (string, error) {
	reasonsJSON, _ := json.Marshal(res.Reasons)
	var id string
	err := r.P.QueryRow(ctx, `
		INSERT INTO ap_health_scores (
		  ap_device_id, ap_score, severity, total_customers,
		  critical_customers, warning_customers, healthy_customers,
		  degradation_ratio, is_ap_wide_interference, reasons, calculated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11) RETURNING id`,
		in.APDeviceID, res.APScore, string(res.Severity),
		len(in.CustomerScores), in.CriticalCustomerCount,
		in.WarningCustomerCount, in.HealthyCustomerCount,
		res.DegradationRatio, res.IsAPWideInterference,
		string(reasonsJSON), res.CalculatedAt).Scan(&id)
	return id, err
}

// LatestAPScore, AP'nin en son sağlık skorunu döner.
func (r *Repository) LatestAPScore(ctx context.Context, apDeviceID string) (map[string]any, error) {
	row := r.P.QueryRow(ctx, `
		SELECT ap_device_id::text, ap_score, severity, total_customers,
		       critical_customers, warning_customers, healthy_customers,
		       degradation_ratio, is_ap_wide_interference, reasons, calculated_at
		FROM ap_health_scores
		WHERE ap_device_id = $1
		ORDER BY calculated_at DESC LIMIT 1`, apDeviceID)
	var (
		dev                               string
		score, total, crit, warn, healthy int
		sev                               string
		ratio                             float64
		apw                               bool
		reasonsRaw                        []byte
		calc                              time.Time
	)
	err := row.Scan(&dev, &score, &sev, &total, &crit, &warn, &healthy, &ratio, &apw, &reasonsRaw, &calc)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var reasons []string
	_ = json.Unmarshal(reasonsRaw, &reasons)
	return map[string]any{
		"ap_device_id":            dev,
		"ap_score":                score,
		"severity":                sev,
		"total_customers":         total,
		"critical_customers":      crit,
		"warning_customers":       warn,
		"healthy_customers":       healthy,
		"degradation_ratio":       ratio,
		"is_ap_wide_interference": apw,
		"reasons":                 reasons,
		"calculated_at":           calc,
	}, nil
}

// SaveTowerScore, tower_risk_scores satırı ekler.
func (r *Repository) SaveTowerScore(ctx context.Context, res TowerResult) (string, error) {
	reasonsJSON, _ := json.Marshal(res.Reasons)
	var id string
	err := r.P.QueryRow(ctx, `
		INSERT INTO tower_risk_scores (tower_id, risk_score, severity, reasons, calculated_at)
		VALUES ($1,$2,$3,$4::jsonb,$5) RETURNING id`,
		res.TowerID, res.RiskScore, string(res.Severity),
		string(reasonsJSON), res.CalculatedAt).Scan(&id)
	return id, err
}

// LatestTowerScore, kulenin son risk skorunu döner.
func (r *Repository) LatestTowerScore(ctx context.Context, towerID string) (map[string]any, error) {
	row := r.P.QueryRow(ctx, `
		SELECT tower_id::text, risk_score, severity, reasons, calculated_at
		FROM tower_risk_scores
		WHERE tower_id = $1
		ORDER BY calculated_at DESC LIMIT 1`, towerID)
	var (
		dev, sev   string
		score      int
		reasonsRaw []byte
		calc       time.Time
	)
	if err := row.Scan(&dev, &score, &sev, &reasonsRaw, &calc); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var reasons []string
	_ = json.Unmarshal(reasonsRaw, &reasons)
	return map[string]any{
		"tower_id":      dev,
		"risk_score":    score,
		"severity":      sev,
		"reasons":       reasons,
		"calculated_at": calc,
	}, nil
}

// ============================================================================
// Work order candidates
// ============================================================================

// WorkOrderCandidateRow, work_order_candidates satırı.
type WorkOrderCandidateRow struct {
	ID                  string    `json:"id"`
	CustomerID          *string   `json:"customer_id,omitempty"`
	APDeviceID          *string   `json:"ap_device_id,omitempty"`
	TowerID             *string   `json:"tower_id,omitempty"`
	SourceScoreID       *string   `json:"source_score_id,omitempty"`
	Diagnosis           string    `json:"diagnosis"`
	RecommendedAction   string    `json:"recommended_action"`
	Severity            string    `json:"severity"`
	Reasons             []string  `json:"reasons"`
	Status              string    `json:"status"`
	Notes               *string   `json:"notes,omitempty"`
	PromotedWorkOrderID *string   `json:"promoted_work_order_id,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// CreateWorkOrderCandidateInput, oluşturma girdisi.
type CreateWorkOrderCandidateInput struct {
	CustomerID        *string
	APDeviceID        *string
	TowerID           *string
	SourceScoreID     *string
	Diagnosis         string
	RecommendedAction string
	Severity          string
	Reasons           []string
	Notes             *string
}

// CreateCandidateOutcome, oluşturma çıktısıdır. Eğer aynı müşteri+tanı için
// halihazırda açık bir aday varsa Duplicate=true, ID o aday döner.
//
// Faz 7 — DuplicateReason değerleri:
//   - "duplicate_open_candidate"  → status=open mevcut
//   - "already_promoted"          → status=promoted, gerçek iş emri var
//   - "recently_dismissed"        → status=dismissed, cooldown içinde
//   - "recently_cancelled"        → status=cancelled, cooldown içinde
type CreateCandidateOutcome struct {
	ID              string
	Duplicate       bool
	DuplicateReason string
	CooldownDays    int
}

// CreateWorkOrderCandidate, yeni iş emri adayı ekler.
//
// Faz 6 — Duplicate guard: aynı customer_id + diagnosis için status='open'
// olan bir aday varsa yenisi oluşturulmaz; mevcut adayın id'si döner ve
// Outcome.Duplicate=true işaretlenir. severity 'critical' / 'warning'
// dışındaysa hata döner (yalnızca sorunlu skorlar aday üretir).
//
// Faz 7 ek: Cooldown genişletmesi.
//
//   - status='open' aday varsa eskisi gibi Duplicate=true döner
//     (DuplicateReason="duplicate_open_candidate").
//   - status='promoted' aday varsa Duplicate=true,
//     DuplicateReason="already_promoted" döner.
//   - status='dismissed' veya 'cancelled' aday varsa, eşik tablosundaki
//     work_order_duplicate_cooldown_days değerine göre updated_at >= now()
//   - cooldown ise Duplicate=true ve DuplicateReason="recently_dismissed"
//     veya "recently_cancelled" döner.
//   - Aksi halde yeni satır eklenir.
func (r *Repository) CreateWorkOrderCandidate(ctx context.Context, in CreateWorkOrderCandidateInput) (CreateCandidateOutcome, error) {
	if in.Severity != string(SeverityWarning) && in.Severity != string(SeverityCritical) {
		return CreateCandidateOutcome{}, errors.New("scoring: only warning/critical severities can create work order candidates")
	}
	if in.Diagnosis == "" {
		return CreateCandidateOutcome{}, errors.New("scoring: diagnosis required")
	}

	cooldownDays := r.LoadDuplicateCooldownDays(ctx)

	if in.CustomerID != nil && *in.CustomerID != "" {
		// Önce open aday — varsa hemen döner.
		var openID string
		err := r.P.QueryRow(ctx, `
			SELECT id::text FROM work_order_candidates
			 WHERE customer_id = $1::uuid
			   AND diagnosis  = $2
			   AND status     = 'open'
			 ORDER BY created_at DESC
			 LIMIT 1`, *in.CustomerID, in.Diagnosis).Scan(&openID)
		if err == nil && openID != "" {
			return CreateCandidateOutcome{
				ID:              openID,
				Duplicate:       true,
				DuplicateReason: "duplicate_open_candidate",
				CooldownDays:    cooldownDays,
			}, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return CreateCandidateOutcome{}, err
		}

		// Promoted bir aday var mı?
		var promotedID string
		var promotedWO *string
		err = r.P.QueryRow(ctx, `
			SELECT id::text, promoted_work_order_id::text FROM work_order_candidates
			 WHERE customer_id = $1::uuid
			   AND diagnosis  = $2
			   AND status     = 'promoted'
			 ORDER BY updated_at DESC
			 LIMIT 1`, *in.CustomerID, in.Diagnosis).Scan(&promotedID, &promotedWO)
		if err == nil && promotedID != "" {
			// Promoted iş emri hâlâ aktif (resolved/cancelled değil) ise yeni
			// aday üretmeyelim — operatör mevcut iş emrini takip etsin.
			if promotedWO != nil && *promotedWO != "" {
				var status string
				err := r.P.QueryRow(ctx,
					`SELECT status FROM work_orders WHERE id = $1::uuid`, *promotedWO).
					Scan(&status)
				if err == nil && status != "resolved" && status != "cancelled" {
					return CreateCandidateOutcome{
						ID:              promotedID,
						Duplicate:       true,
						DuplicateReason: "already_promoted",
						CooldownDays:    cooldownDays,
					}, nil
				}
			}
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return CreateCandidateOutcome{}, err
		}

		// Cooldown — dismissed / cancelled.
		if cooldownDays > 0 {
			var recentID, recentStatus string
			err = r.P.QueryRow(ctx, `
				SELECT id::text, status FROM work_order_candidates
				 WHERE customer_id = $1::uuid
				   AND diagnosis  = $2
				   AND status IN ('dismissed','cancelled')
				   AND updated_at >= now() - ($3 || ' days')::interval
				 ORDER BY updated_at DESC
				 LIMIT 1`, *in.CustomerID, in.Diagnosis, cooldownDays).
				Scan(&recentID, &recentStatus)
			if err == nil && recentID != "" {
				reason := "recently_dismissed"
				if recentStatus == "cancelled" {
					reason = "recently_cancelled"
				}
				return CreateCandidateOutcome{
					ID:              recentID,
					Duplicate:       true,
					DuplicateReason: reason,
					CooldownDays:    cooldownDays,
				}, nil
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return CreateCandidateOutcome{}, err
			}
		}
	}

	reasonsJSON, _ := json.Marshal(in.Reasons)
	var id string
	err := r.P.QueryRow(ctx, `
		INSERT INTO work_order_candidates (
		  customer_id, ap_device_id, tower_id, source_score_id,
		  diagnosis, recommended_action, severity, reasons, notes
		) VALUES (
		  NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, NULLIF($4,'')::uuid,
		  $5, $6, $7, $8::jsonb, $9
		) RETURNING id`,
		strOrEmpty(in.CustomerID), strOrEmpty(in.APDeviceID),
		strOrEmpty(in.TowerID), strOrEmpty(in.SourceScoreID),
		in.Diagnosis, in.RecommendedAction, in.Severity,
		string(reasonsJSON), in.Notes).Scan(&id)
	if err != nil {
		return CreateCandidateOutcome{}, err
	}
	return CreateCandidateOutcome{ID: id, Duplicate: false, CooldownDays: cooldownDays}, nil
}

// LoadDuplicateCooldownDays, scoring_thresholds tablosundan
// work_order_duplicate_cooldown_days değerini okur. Bulamazsa 7 döner
// (Faz 7 varsayılanı).
func (r *Repository) LoadDuplicateCooldownDays(ctx context.Context) int {
	if r == nil || r.P == nil {
		return 7
	}
	var v float64
	err := r.P.QueryRow(ctx,
		`SELECT value FROM scoring_thresholds WHERE key = 'work_order_duplicate_cooldown_days'`).
		Scan(&v)
	if err != nil {
		return 7
	}
	if v < 0 {
		return 0
	}
	return int(v)
}

// ListWorkOrderCandidates, açık (open) adayları döner.
func (r *Repository) ListWorkOrderCandidates(ctx context.Context, status string, limit int) ([]WorkOrderCandidateRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if status == "" {
		status = "open"
	}
	rows, err := r.P.Query(ctx, `
		SELECT id, customer_id::text, ap_device_id::text, tower_id::text,
		       source_score_id::text, diagnosis, recommended_action, severity,
		       reasons, status, notes, promoted_work_order_id::text,
		       created_at, updated_at
		FROM work_order_candidates
		WHERE status = $1
		ORDER BY
		  CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
		  created_at DESC
		LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkOrderCandidateRow{}
	for rows.Next() {
		var w WorkOrderCandidateRow
		var reasonsRaw []byte
		if err := rows.Scan(&w.ID, &w.CustomerID, &w.APDeviceID, &w.TowerID,
			&w.SourceScoreID, &w.Diagnosis, &w.RecommendedAction, &w.Severity,
			&reasonsRaw, &w.Status, &w.Notes, &w.PromotedWorkOrderID,
			&w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reasonsRaw, &w.Reasons)
		out = append(out, w)
	}
	return out, rows.Err()
}

// UpdateCandidateStatus, "open" → "dismissed" / "promoted" / "cancelled".
func (r *Repository) UpdateCandidateStatus(ctx context.Context, id, status string, notes *string) error {
	if status != "open" && status != "dismissed" && status != "promoted" && status != "cancelled" {
		return errors.New("scoring: invalid candidate status")
	}
	tag, err := r.P.Exec(ctx, `
		UPDATE work_order_candidates
		   SET status = $2, notes = COALESCE($3, notes), updated_at = now()
		 WHERE id = $1`, id, status, notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func scanCustomerScore(row pgx.Row) (*CustomerScoreRow, error) {
	var r CustomerScoreRow
	var reasonsRaw, cmRaw []byte
	if err := row.Scan(
		&r.ID, &r.CustomerID, &r.APDeviceID, &r.TowerID,
		&r.Score, &r.Severity, &r.Diagnosis, &r.RecommendedAction,
		&reasonsRaw, &cmRaw,
		&r.RSSIdBm, &r.SNRdB, &r.CCQ, &r.PacketLossPct,
		&r.AvgLatencyMs, &r.JitterMs,
		&r.SignalTrend7d, &r.IsStale, &r.CalculatedAt,
	); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(reasonsRaw, &r.Reasons)
	_ = json.Unmarshal(cmRaw, &r.ContributingMetrics)
	return &r, nil
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func itoa(n int) string {
	// küçük yardımcı; strconv.Itoa kullanmayı tercih eden yerler için
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func joinAnd(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}
