package reports

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgUndefinedTable, Postgres SQLSTATE 42P01 (relation does not exist).
// BuildExecutiveSummary çağrılırken Phase 7 migration uygulanmadıysa
// work_orders tablosu yoktur; bu durumda iş emri sayaçlarını sıfırda
// bırakıp diğer aggregate'leri döndürmeye devam ederiz.
const pgUndefinedTable = "42P01"

// Repository, executive summary ve detay rapor sorgularını çalıştırır.
type Repository struct {
	P *pgxpool.Pool
}

// NewRepository, pgxpool ile yeni bir repo döndürür.
func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{P: p} }

// ProblemCustomerRow, /reports/problem-customers + CSV satırı.
type ProblemCustomerRow struct {
	CustomerID        string    `json:"customer_id"`
	CustomerName      string    `json:"customer_name"`
	APDeviceID        *string   `json:"ap_device_id,omitempty"`
	APDeviceName      *string   `json:"ap_device_name,omitempty"`
	TowerID           *string   `json:"tower_id,omitempty"`
	TowerName         *string   `json:"tower_name,omitempty"`
	Score             int       `json:"score"`
	Severity          string    `json:"severity"`
	Diagnosis         string    `json:"diagnosis"`
	RecommendedAction string    `json:"recommended_action"`
	IsStale           bool      `json:"is_stale"`
	CalculatedAt      time.Time `json:"calculated_at"`
}

// APHealthRow, /reports/ap-health + CSV satırı.
type APHealthRow struct {
	APDeviceID         string    `json:"ap_device_id"`
	APDeviceName       string    `json:"ap_device_name"`
	TowerID            *string   `json:"tower_id,omitempty"`
	TowerName          *string   `json:"tower_name,omitempty"`
	APScore            int       `json:"ap_score"`
	Severity           string    `json:"severity"`
	TotalCustomers     int       `json:"total_customers"`
	CriticalCustomers  int       `json:"critical_customers"`
	WarningCustomers   int       `json:"warning_customers"`
	HealthyCustomers   int       `json:"healthy_customers"`
	DegradationRatio   float64   `json:"degradation_ratio"`
	APWideInterference bool      `json:"is_ap_wide_interference"`
	CalculatedAt       time.Time `json:"calculated_at"`
}

// TowerRiskRow, /reports/tower-risk + CSV satırı.
type TowerRiskRow struct {
	TowerID      string    `json:"tower_id"`
	TowerName    string    `json:"tower_name"`
	RiskScore    int       `json:"risk_score"`
	Severity     string    `json:"severity"`
	CalculatedAt time.Time `json:"calculated_at"`
}

// WorkOrderRow, /reports/work-orders + CSV satırı (özet).
type WorkOrderRow struct {
	ID                string     `json:"id"`
	Title             string     `json:"title"`
	CustomerID        *string    `json:"customer_id,omitempty"`
	CustomerName      *string    `json:"customer_name,omitempty"`
	APDeviceID        *string    `json:"ap_device_id,omitempty"`
	APDeviceName      *string    `json:"ap_device_name,omitempty"`
	TowerID           *string    `json:"tower_id,omitempty"`
	TowerName         *string    `json:"tower_name,omitempty"`
	Diagnosis         string     `json:"diagnosis"`
	RecommendedAction string     `json:"recommended_action"`
	Severity          string     `json:"severity"`
	Status            string     `json:"status"`
	Priority          string     `json:"priority"`
	AssignedTo        *string    `json:"assigned_to,omitempty"`
	ETAAt             *time.Time `json:"eta_at,omitempty"`
	OverdueETA        bool       `json:"overdue_eta"`
	CreatedAt         time.Time  `json:"created_at"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
}

// DiagnosisCount, en çok tekrar eden tanı satırı.
type DiagnosisCount struct {
	Diagnosis string `json:"diagnosis"`
	Count     int    `json:"count"`
}

// TrendBucket, severity bazında günlük trend.
type TrendBucket struct {
	Day      time.Time `json:"day"`
	Critical int       `json:"critical"`
	Warning  int       `json:"warning"`
	Healthy  int       `json:"healthy"`
}

// ExecutiveSummary, /reports/executive-summary cevap gövdesi.
type ExecutiveSummary struct {
	GeneratedAt time.Time `json:"generated_at"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`

	TotalCustomers    int `json:"total_customers"`
	CriticalCustomers int `json:"critical_customers"`
	WarningCustomers  int `json:"warning_customers"`
	StaleCustomers    int `json:"stale_customers"`
	APWideInterAffect int `json:"ap_wide_interference_customers"`

	Top10RiskyAPs    []APHealthRow    `json:"top10_risky_aps"`
	Top10RiskyTowers []TowerRiskRow   `json:"top10_risky_towers"`
	Top10Diagnoses   []DiagnosisCount `json:"top10_diagnoses"`

	OpenWorkOrders   int `json:"open_work_orders"`
	UrgentOrHighPrio int `json:"urgent_or_high_priority_work_orders"`
	OverdueETA       int `json:"overdue_eta_work_orders"`
	CreatedToday     int `json:"created_today_work_orders"`

	Trend7d  []TrendBucket `json:"trend_7d"`
	Trend30d []TrendBucket `json:"trend_30d"`
}

// ReportsFilter, rapor endpointleri için ortak filtreler.
type ReportsFilter struct {
	Severity   string
	Diagnosis  string
	TowerID    string
	APDeviceID string
	CustomerID string
	Status     string
	Priority   string
	AssignedTo string
	DateFrom   *time.Time
	DateTo     *time.Time
	Limit      int
	OnlyStale  bool
}

// =============================================================================
// Executive summary
// =============================================================================

// BuildExecutiveSummary, son skor satırlarına göre yönetici özeti üretir.
//
// Faz 7 — yalnız okuma yapar; cihazla konuşmaz, skor üretmez. PeriodEnd
// = now() ve PeriodStart = now() - 30 gün varsayılır.
func (r *Repository) BuildExecutiveSummary(ctx context.Context) (ExecutiveSummary, error) {
	now := time.Now().UTC()
	es := ExecutiveSummary{
		GeneratedAt: now,
		PeriodStart: now.AddDate(0, 0, -30),
		PeriodEnd:   now,
	}

	// Toplam müşteri.
	if err := r.P.QueryRow(ctx, `SELECT COUNT(*) FROM customers WHERE status = 'active'`).
		Scan(&es.TotalCustomers); err != nil {
		return es, err
	}

	// Severity breakdown — yalnız her müşterinin SON skor satırı.
	row := r.P.QueryRow(ctx, `
WITH latest AS (
  SELECT DISTINCT ON (customer_id)
         customer_id, severity, diagnosis, is_stale, calculated_at
    FROM customer_signal_scores
   ORDER BY customer_id, calculated_at DESC
)
SELECT
  COUNT(*) FILTER (WHERE severity = 'critical')                  AS critical,
  COUNT(*) FILTER (WHERE severity = 'warning')                   AS warning,
  COUNT(*) FILTER (WHERE is_stale = true)                        AS stale,
  COUNT(*) FILTER (WHERE diagnosis = 'ap_wide_interference')     AS ap_wide
  FROM latest`)
	if err := row.Scan(&es.CriticalCustomers, &es.WarningCustomers,
		&es.StaleCustomers, &es.APWideInterAffect); err != nil {
		return es, err
	}

	// Top 10 risky APs — en yeni ap_health_scores satırları, score asc.
	aps, err := r.TopRiskyAPs(ctx, 10)
	if err != nil {
		return es, err
	}
	es.Top10RiskyAPs = aps

	// Top 10 risky towers.
	towers, err := r.TopRiskyTowers(ctx, 10)
	if err != nil {
		return es, err
	}
	es.Top10RiskyTowers = towers

	// Top diagnoses (son skor satırı bazında).
	diag, err := r.TopDiagnoses(ctx, 10)
	if err != nil {
		return es, err
	}
	es.Top10Diagnoses = diag

	// İş emri sayaçları. Phase 7 migration uygulanmadıysa work_orders
	// tablosu olmayabilir → SQLSTATE 42P01 ise sıfır bırakıp devam et.
	woRow := r.P.QueryRow(ctx, `
SELECT
  COUNT(*) FILTER (WHERE status NOT IN ('resolved','cancelled'))                                            AS open_total,
  COUNT(*) FILTER (WHERE priority IN ('urgent','high') AND status NOT IN ('resolved','cancelled'))          AS urgent_high,
  COUNT(*) FILTER (WHERE eta_at IS NOT NULL AND eta_at < now() AND status NOT IN ('resolved','cancelled'))  AS overdue,
  COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now()))                                            AS today
FROM work_orders`)
	if err := woRow.Scan(&es.OpenWorkOrders, &es.UrgentOrHighPrio,
		&es.OverdueETA, &es.CreatedToday); err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != pgUndefinedTable {
			return es, err
		}
	}

	// Trend 7d / 30d.
	t7, err := r.severityTrend(ctx, 7)
	if err != nil {
		return es, err
	}
	es.Trend7d = t7
	t30, err := r.severityTrend(ctx, 30)
	if err != nil {
		return es, err
	}
	es.Trend30d = t30

	return es, nil
}

// severityTrend, son N gün için günlük critical/warning/healthy sayımı.
func (r *Repository) severityTrend(ctx context.Context, days int) ([]TrendBucket, error) {
	rows, err := r.P.Query(ctx, `
SELECT date_trunc('day', calculated_at) AS day,
       COUNT(*) FILTER (WHERE severity = 'critical') AS critical,
       COUNT(*) FILTER (WHERE severity = 'warning')  AS warning,
       COUNT(*) FILTER (WHERE severity = 'healthy')  AS healthy
  FROM customer_signal_scores
 WHERE calculated_at >= now() - make_interval(days => $1::int)
 GROUP BY day
 ORDER BY day ASC`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrendBucket{}
	for rows.Next() {
		var b TrendBucket
		if err := rows.Scan(&b.Day, &b.Critical, &b.Warning, &b.Healthy); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// TopRiskyAPs, en yeni ap_health_scores satırlarından düşük score'lu N tane.
func (r *Repository) TopRiskyAPs(ctx context.Context, limit int) ([]APHealthRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 10
	}
	rows, err := r.P.Query(ctx, `
WITH latest AS (
  SELECT DISTINCT ON (ap_device_id) ap_device_id, ap_score, severity,
                                    total_customers, critical_customers, warning_customers,
                                    healthy_customers, degradation_ratio, is_ap_wide_interference,
                                    calculated_at
    FROM ap_health_scores
   ORDER BY ap_device_id, calculated_at DESC
)
SELECT l.ap_device_id::text, COALESCE(d.name,''),
       d.tower_id::text, t.name,
       l.ap_score, l.severity, l.total_customers, l.critical_customers,
       l.warning_customers, l.healthy_customers, l.degradation_ratio,
       l.is_ap_wide_interference, l.calculated_at
  FROM latest l
  LEFT JOIN devices d ON d.id = l.ap_device_id
  LEFT JOIN towers  t ON t.id = d.tower_id
 ORDER BY
   CASE l.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
   l.ap_score ASC,
   l.calculated_at DESC
 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APHealthRow{}
	for rows.Next() {
		var a APHealthRow
		if err := rows.Scan(&a.APDeviceID, &a.APDeviceName,
			&a.TowerID, &a.TowerName,
			&a.APScore, &a.Severity, &a.TotalCustomers, &a.CriticalCustomers,
			&a.WarningCustomers, &a.HealthyCustomers, &a.DegradationRatio,
			&a.APWideInterference, &a.CalculatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TopRiskyTowers, en yeni tower_risk_scores satırlarından N tane.
func (r *Repository) TopRiskyTowers(ctx context.Context, limit int) ([]TowerRiskRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 10
	}
	rows, err := r.P.Query(ctx, `
WITH latest AS (
  SELECT DISTINCT ON (tower_id) tower_id, risk_score, severity, calculated_at
    FROM tower_risk_scores
   ORDER BY tower_id, calculated_at DESC
)
SELECT l.tower_id::text, COALESCE(t.name,''), l.risk_score, l.severity, l.calculated_at
  FROM latest l
  LEFT JOIN towers t ON t.id = l.tower_id
 ORDER BY
   CASE l.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
   l.risk_score ASC,
   l.calculated_at DESC
 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TowerRiskRow{}
	for rows.Next() {
		var t TowerRiskRow
		if err := rows.Scan(&t.TowerID, &t.TowerName, &t.RiskScore,
			&t.Severity, &t.CalculatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TopDiagnoses, son skor satırı bazında en çok tekrar eden tanılar.
func (r *Repository) TopDiagnoses(ctx context.Context, limit int) ([]DiagnosisCount, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := r.P.Query(ctx, `
WITH latest AS (
  SELECT DISTINCT ON (customer_id) customer_id, diagnosis, severity
    FROM customer_signal_scores
   ORDER BY customer_id, calculated_at DESC
)
SELECT diagnosis, COUNT(*)
  FROM latest
 WHERE severity IN ('warning','critical')
 GROUP BY diagnosis
 ORDER BY COUNT(*) DESC, diagnosis ASC
 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DiagnosisCount{}
	for rows.Next() {
		var d DiagnosisCount
		if err := rows.Scan(&d.Diagnosis, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// =============================================================================
// Detailed reports
// =============================================================================

// ProblemCustomers, /reports/problem-customers gövdesi.
func (r *Repository) ProblemCustomers(ctx context.Context, f ReportsFilter) ([]ProblemCustomerRow, error) {
	if f.Limit <= 0 || f.Limit > 5000 {
		f.Limit = 1000
	}
	conds := []string{"css.id = (SELECT id FROM customer_signal_scores WHERE customer_id = c.id ORDER BY calculated_at DESC LIMIT 1)"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, cond+"$"+strconv.Itoa(len(args)))
	}
	if f.Severity != "" {
		add("css.severity = ", f.Severity)
	} else {
		conds = append(conds, "css.severity IN ('warning','critical')")
	}
	if f.Diagnosis != "" {
		add("css.diagnosis = ", f.Diagnosis)
	}
	if f.TowerID != "" {
		add("css.tower_id::text = ", f.TowerID)
	}
	if f.APDeviceID != "" {
		add("css.ap_device_id::text = ", f.APDeviceID)
	}
	if f.OnlyStale {
		conds = append(conds, "css.is_stale = true")
	}
	if f.DateFrom != nil {
		add("css.calculated_at >= ", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("css.calculated_at <= ", *f.DateTo)
	}
	args = append(args, f.Limit)
	limitArg := strconv.Itoa(len(args))

	q := `SELECT c.id::text, c.full_name,
                 css.ap_device_id::text, COALESCE(d.name,''),
                 css.tower_id::text, COALESCE(t.name,''),
                 css.score, css.severity, css.diagnosis, css.recommended_action,
                 css.is_stale, css.calculated_at
            FROM customers c
            JOIN customer_signal_scores css ON css.customer_id = c.id
       LEFT JOIN devices d ON d.id = css.ap_device_id
       LEFT JOIN towers  t ON t.id = css.tower_id
           WHERE ` + strings.Join(conds, " AND ") + `
        ORDER BY
          CASE css.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
          css.score ASC, css.calculated_at DESC
        LIMIT $` + limitArg
	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProblemCustomerRow{}
	for rows.Next() {
		var p ProblemCustomerRow
		var apName, towName string
		if err := rows.Scan(&p.CustomerID, &p.CustomerName,
			&p.APDeviceID, &apName,
			&p.TowerID, &towName,
			&p.Score, &p.Severity, &p.Diagnosis, &p.RecommendedAction,
			&p.IsStale, &p.CalculatedAt); err != nil {
			return nil, err
		}
		if apName != "" {
			p.APDeviceName = &apName
		}
		if towName != "" {
			p.TowerName = &towName
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// APHealth, /reports/ap-health gövdesi.
func (r *Repository) APHealth(ctx context.Context, f ReportsFilter) ([]APHealthRow, error) {
	if f.Limit <= 0 || f.Limit > 5000 {
		f.Limit = 1000
	}
	conds := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, cond+"$"+strconv.Itoa(len(args)))
	}
	if f.Severity != "" {
		add("l.severity = ", f.Severity)
	}
	if f.TowerID != "" {
		add("d.tower_id::text = ", f.TowerID)
	}
	if f.DateFrom != nil {
		add("l.calculated_at >= ", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("l.calculated_at <= ", *f.DateTo)
	}
	args = append(args, f.Limit)
	limitArg := strconv.Itoa(len(args))

	q := `WITH latest AS (
  SELECT DISTINCT ON (ap_device_id)
         ap_device_id, ap_score, severity, total_customers, critical_customers,
         warning_customers, healthy_customers, degradation_ratio,
         is_ap_wide_interference, calculated_at
    FROM ap_health_scores
   ORDER BY ap_device_id, calculated_at DESC
)
SELECT l.ap_device_id::text, COALESCE(d.name,''),
       d.tower_id::text, t.name,
       l.ap_score, l.severity, l.total_customers, l.critical_customers,
       l.warning_customers, l.healthy_customers, l.degradation_ratio,
       l.is_ap_wide_interference, l.calculated_at
  FROM latest l
  LEFT JOIN devices d ON d.id = l.ap_device_id
  LEFT JOIN towers  t ON t.id = d.tower_id
 WHERE ` + strings.Join(conds, " AND ") + `
 ORDER BY
   CASE l.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
   l.ap_score ASC, l.calculated_at DESC
 LIMIT $` + limitArg
	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APHealthRow{}
	for rows.Next() {
		var a APHealthRow
		if err := rows.Scan(&a.APDeviceID, &a.APDeviceName,
			&a.TowerID, &a.TowerName,
			&a.APScore, &a.Severity, &a.TotalCustomers, &a.CriticalCustomers,
			&a.WarningCustomers, &a.HealthyCustomers, &a.DegradationRatio,
			&a.APWideInterference, &a.CalculatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TowerRisk, /reports/tower-risk gövdesi.
func (r *Repository) TowerRisk(ctx context.Context, f ReportsFilter) ([]TowerRiskRow, error) {
	if f.Limit <= 0 || f.Limit > 5000 {
		f.Limit = 1000
	}
	conds := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, cond+"$"+strconv.Itoa(len(args)))
	}
	if f.Severity != "" {
		add("l.severity = ", f.Severity)
	}
	if f.DateFrom != nil {
		add("l.calculated_at >= ", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("l.calculated_at <= ", *f.DateTo)
	}
	args = append(args, f.Limit)
	limitArg := strconv.Itoa(len(args))
	q := `WITH latest AS (
  SELECT DISTINCT ON (tower_id) tower_id, risk_score, severity, calculated_at
    FROM tower_risk_scores
   ORDER BY tower_id, calculated_at DESC
)
SELECT l.tower_id::text, COALESCE(t.name,''), l.risk_score, l.severity, l.calculated_at
  FROM latest l
  LEFT JOIN towers t ON t.id = l.tower_id
 WHERE ` + strings.Join(conds, " AND ") + `
 ORDER BY
   CASE l.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
   l.risk_score ASC, l.calculated_at DESC
 LIMIT $` + limitArg
	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TowerRiskRow{}
	for rows.Next() {
		var t TowerRiskRow
		if err := rows.Scan(&t.TowerID, &t.TowerName, &t.RiskScore,
			&t.Severity, &t.CalculatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// WorkOrdersReport, /reports/work-orders gövdesi.
func (r *Repository) WorkOrdersReport(ctx context.Context, f ReportsFilter) ([]WorkOrderRow, error) {
	if f.Limit <= 0 || f.Limit > 5000 {
		f.Limit = 1000
	}
	conds := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, cond+"$"+strconv.Itoa(len(args)))
	}
	if f.Status != "" {
		add("wo.status = ", f.Status)
	}
	if f.Priority != "" {
		add("wo.priority = ", f.Priority)
	}
	if f.Severity != "" {
		add("wo.severity = ", f.Severity)
	}
	if f.TowerID != "" {
		add("wo.tower_id::text = ", f.TowerID)
	}
	if f.APDeviceID != "" {
		add("wo.ap_device_id::text = ", f.APDeviceID)
	}
	if f.CustomerID != "" {
		add("wo.customer_id::text = ", f.CustomerID)
	}
	if f.AssignedTo != "" {
		add("wo.assigned_to = ", f.AssignedTo)
	}
	if f.DateFrom != nil {
		add("wo.created_at >= ", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("wo.created_at <= ", *f.DateTo)
	}
	args = append(args, f.Limit)
	limitArg := strconv.Itoa(len(args))

	q := `SELECT wo.id::text, wo.title,
       wo.customer_id::text, c.full_name,
       wo.ap_device_id::text, d.name,
       wo.tower_id::text, t.name,
       wo.diagnosis, wo.recommended_action, wo.severity,
       wo.status, wo.priority, wo.assigned_to,
       wo.eta_at, wo.created_at, wo.resolved_at
  FROM work_orders wo
  LEFT JOIN customers c ON c.id = wo.customer_id
  LEFT JOIN devices   d ON d.id = wo.ap_device_id
  LEFT JOIN towers    t ON t.id = wo.tower_id
 WHERE ` + strings.Join(conds, " AND ") + `
 ORDER BY
   CASE wo.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1
                    WHEN 'medium' THEN 2 ELSE 3 END,
   CASE wo.status   WHEN 'open' THEN 0 WHEN 'assigned' THEN 1
                    WHEN 'in_progress' THEN 2 WHEN 'cancelled' THEN 3
                    WHEN 'resolved' THEN 4 ELSE 5 END,
   wo.created_at DESC
 LIMIT $` + limitArg
	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	out := []WorkOrderRow{}
	for rows.Next() {
		var w WorkOrderRow
		var custName, apName, towName *string
		if err := rows.Scan(&w.ID, &w.Title,
			&w.CustomerID, &custName,
			&w.APDeviceID, &apName,
			&w.TowerID, &towName,
			&w.Diagnosis, &w.RecommendedAction, &w.Severity,
			&w.Status, &w.Priority, &w.AssignedTo,
			&w.ETAAt, &w.CreatedAt, &w.ResolvedAt); err != nil {
			return nil, err
		}
		w.CustomerName = custName
		w.APDeviceName = apName
		w.TowerName = towName
		if w.ETAAt != nil && w.ETAAt.Before(now) &&
			w.Status != "resolved" && w.Status != "cancelled" {
			w.OverdueETA = true
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// =============================================================================
// Snapshots
// =============================================================================

// SaveSnapshot, report_snapshots tablosuna bir satır ekler.
func (r *Repository) SaveSnapshot(ctx context.Context, reportType string,
	periodStart, periodEnd time.Time, payload any, generatedBy string) (string, error) {
	if generatedBy == "" {
		generatedBy = "system"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var id string
	err = r.P.QueryRow(ctx, `
INSERT INTO report_snapshots(report_type, period_start, period_end, payload, generated_by)
VALUES ($1,$2,$3,$4::jsonb,$5)
RETURNING id::text`, reportType, periodStart, periodEnd, string(body), generatedBy).Scan(&id)
	return id, err
}

// SnapshotRow, report_snapshots listesi satırı.
type SnapshotRow struct {
	ID          string          `json:"id"`
	ReportType  string          `json:"report_type"`
	PeriodStart time.Time       `json:"period_start"`
	PeriodEnd   time.Time       `json:"period_end"`
	GeneratedAt time.Time       `json:"generated_at"`
	GeneratedBy string          `json:"generated_by"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// ListSnapshots, son snapshot'ları döner.
func (r *Repository) ListSnapshots(ctx context.Context, reportType string, limit int) ([]SnapshotRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	conds := []string{"1=1"}
	args := []any{}
	if reportType != "" {
		args = append(args, reportType)
		conds = append(conds, "report_type = $1")
	}
	args = append(args, limit)
	limitArg := strconv.Itoa(len(args))
	q := `SELECT id::text, report_type, period_start, period_end, generated_at, generated_by, payload
            FROM report_snapshots
           WHERE ` + strings.Join(conds, " AND ") + `
        ORDER BY generated_at DESC
           LIMIT $` + limitArg
	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SnapshotRow{}
	for rows.Next() {
		var s SnapshotRow
		var payloadBytes []byte
		if err := rows.Scan(&s.ID, &s.ReportType, &s.PeriodStart, &s.PeriodEnd,
			&s.GeneratedAt, &s.GeneratedBy, &payloadBytes); err != nil {
			return nil, err
		}
		s.Payload = payloadBytes
		out = append(out, s)
	}
	return out, rows.Err()
}
