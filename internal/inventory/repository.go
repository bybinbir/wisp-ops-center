package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository wraps the pgx pool with concrete CRUD methods used by
// the HTTP handlers.
type Repository struct {
	P *pgxpool.Pool
}

func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{P: p} }

// -------- Devices ----------------------------------------------

const deviceCols = `id, name, vendor, role, COALESCE(host(ip),''), site_id::text, tower_id::text,
COALESCE(model,''), COALESCE(os_version,''), COALESCE(firmware_version,''),
status, COALESCE(tags,'{}'), COALESCE(notes,''), created_at, updated_at`

func scanDevice(row pgx.Row) (*Device, error) {
	var d Device
	var siteID, towerID *string
	if err := row.Scan(
		&d.ID, &d.Name, &d.Vendor, &d.Role, &d.IPAddress, &siteID, &towerID,
		&d.Model, &d.OSVersion, &d.FirmwareVersion,
		&d.Status, &d.Tags, &d.Notes, &d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return nil, err
	}
	d.SiteID = siteID
	d.TowerID = towerID
	if d.Tags == nil {
		d.Tags = []string{}
	}
	return &d, nil
}

func (r *Repository) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := r.P.Query(ctx, `SELECT `+deviceCols+`
FROM devices WHERE deleted_at IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Device, 0)
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (r *Repository) GetDevice(ctx context.Context, id string) (*Device, error) {
	row := r.P.QueryRow(ctx, `SELECT `+deviceCols+`
FROM devices WHERE id = $1 AND deleted_at IS NULL`, id)
	d, err := scanDevice(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// CreateDeviceInput is a partial Device.
type CreateDeviceInput struct {
	Name            string
	Vendor          string
	Role            string
	IPAddress       string
	SiteID          *string
	TowerID         *string
	Model           string
	OSVersion       string
	FirmwareVersion string
	Status          string
	Tags            []string
	Notes           string
}

func (r *Repository) CreateDevice(ctx context.Context, in CreateDeviceInput) (*Device, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, &ErrValidation{Msg: "name is required"}
	}
	if !ValidateVendor(in.Vendor) {
		return nil, &ErrValidation{Msg: "vendor must be mikrotik|mimosa|other|unknown"}
	}
	if !ValidateRole(in.Role) {
		return nil, &ErrValidation{Msg: "role must be ap|cpe|ptp_master|ptp_slave|router|switch"}
	}
	if !ValidIP(in.IPAddress) {
		return nil, &ErrValidation{Msg: "ip_address is not a valid IP"}
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if !ValidateDeviceStatus(in.Status) {
		return nil, &ErrValidation{Msg: "status must be active|retired|maintenance|spare"}
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}

	var ipParam interface{}
	if in.IPAddress == "" {
		ipParam = nil
	} else {
		ipParam = in.IPAddress
	}

	row := r.P.QueryRow(ctx, `
INSERT INTO devices(name, vendor, role, ip, site_id, tower_id, model, os_version, firmware_version, status, tags, notes)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING `+deviceCols,
		in.Name, strings.ToLower(in.Vendor), strings.ToLower(in.Role),
		ipParam, in.SiteID, in.TowerID,
		nilIfEmpty(in.Model), nilIfEmpty(in.OSVersion), nilIfEmpty(in.FirmwareVersion),
		in.Status, in.Tags, nilIfEmpty(in.Notes),
	)
	return scanDevice(row)
}

// UpdateDeviceInput allows partial updates.
type UpdateDeviceInput struct {
	Name            *string
	Vendor          *string
	Role            *string
	IPAddress       *string
	SiteID          *string
	TowerID         *string
	Model           *string
	OSVersion       *string
	FirmwareVersion *string
	Status          *string
	Tags            *[]string
	Notes           *string
}

func (r *Repository) UpdateDevice(ctx context.Context, id string, in UpdateDeviceInput) (*Device, error) {
	// Validate transitions when present.
	if in.Vendor != nil && !ValidateVendor(*in.Vendor) {
		return nil, &ErrValidation{Msg: "vendor invalid"}
	}
	if in.Role != nil && !ValidateRole(*in.Role) {
		return nil, &ErrValidation{Msg: "role invalid"}
	}
	if in.Status != nil && !ValidateDeviceStatus(*in.Status) {
		return nil, &ErrValidation{Msg: "status invalid"}
	}
	if in.IPAddress != nil && !ValidIP(*in.IPAddress) {
		return nil, &ErrValidation{Msg: "ip_address invalid"}
	}

	q := `UPDATE devices SET updated_at = now()`
	args := []any{}
	add := func(col string, val any) {
		args = append(args, val)
		q += ", " + col + " = $" + itoa(len(args))
	}
	if in.Name != nil {
		add("name", *in.Name)
	}
	if in.Vendor != nil {
		add("vendor", strings.ToLower(*in.Vendor))
	}
	if in.Role != nil {
		add("role", strings.ToLower(*in.Role))
	}
	if in.IPAddress != nil {
		if *in.IPAddress == "" {
			add("ip", nil)
		} else {
			add("ip", *in.IPAddress)
		}
	}
	if in.SiteID != nil {
		add("site_id", *in.SiteID)
	}
	if in.TowerID != nil {
		add("tower_id", *in.TowerID)
	}
	if in.Model != nil {
		add("model", *in.Model)
	}
	if in.OSVersion != nil {
		add("os_version", *in.OSVersion)
	}
	if in.FirmwareVersion != nil {
		add("firmware_version", *in.FirmwareVersion)
	}
	if in.Status != nil {
		add("status", *in.Status)
	}
	if in.Tags != nil {
		add("tags", *in.Tags)
	}
	if in.Notes != nil {
		add("notes", *in.Notes)
	}

	args = append(args, id)
	q += " WHERE id = $" + itoa(len(args)) + " AND deleted_at IS NULL RETURNING " + deviceCols
	d, err := scanDevice(r.P.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// DeleteDevice performs a soft delete; audit history is preserved.
func (r *Repository) DeleteDevice(ctx context.Context, id string) error {
	cmd, err := r.P.Exec(ctx, `UPDATE devices SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// -------- Sites ------------------------------------------------

func (r *Repository) ListSites(ctx context.Context) ([]Site, error) {
	rows, err := r.P.Query(ctx, `SELECT id, name, COALESCE(code,''), COALESCE(region,''), COALESCE(address,''), latitude, longitude, COALESCE(notes,''), created_at, updated_at FROM sites ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Site, 0)
	for rows.Next() {
		var s Site
		if err := rows.Scan(&s.ID, &s.Name, &s.Code, &s.Region, &s.Address, &s.Latitude, &s.Longitude, &s.Notes, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type CreateSiteInput struct {
	Name      string
	Code      string
	Region    string
	Address   string
	Latitude  *float64
	Longitude *float64
	Notes     string
}

func (r *Repository) CreateSite(ctx context.Context, in CreateSiteInput) (*Site, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, &ErrValidation{Msg: "name is required"}
	}
	var s Site
	err := r.P.QueryRow(ctx, `
INSERT INTO sites(name, code, region, address, latitude, longitude, notes)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, name, COALESCE(code,''), COALESCE(region,''), COALESCE(address,''), latitude, longitude, COALESCE(notes,''), created_at, updated_at`,
		in.Name, nilIfEmpty(in.Code), nilIfEmpty(in.Region), nilIfEmpty(in.Address),
		in.Latitude, in.Longitude, nilIfEmpty(in.Notes),
	).Scan(&s.ID, &s.Name, &s.Code, &s.Region, &s.Address, &s.Latitude, &s.Longitude, &s.Notes, &s.CreatedAt, &s.UpdatedAt)
	return &s, err
}

// -------- Towers -----------------------------------------------

func (r *Repository) ListTowers(ctx context.Context) ([]Tower, error) {
	rows, err := r.P.Query(ctx, `SELECT id, site_id::text, name, COALESCE(code,''), height_m, latitude, longitude, COALESCE(notes,''), created_at, updated_at FROM towers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Tower, 0)
	for rows.Next() {
		var t Tower
		var siteID *string
		var height *float64
		if err := rows.Scan(&t.ID, &siteID, &t.Name, &t.Code, &height, &t.Latitude, &t.Longitude, &t.Notes, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.SiteID = siteID
		t.HeightM = height
		out = append(out, t)
	}
	return out, rows.Err()
}

type CreateTowerInput struct {
	SiteID    *string
	Name      string
	Code      string
	HeightM   *float64
	Latitude  *float64
	Longitude *float64
	Notes     string
}

func (r *Repository) CreateTower(ctx context.Context, in CreateTowerInput) (*Tower, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, &ErrValidation{Msg: "name is required"}
	}
	var t Tower
	var siteID *string
	var height *float64
	err := r.P.QueryRow(ctx, `
INSERT INTO towers(site_id, name, code, height_m, latitude, longitude, notes)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, site_id::text, name, COALESCE(code,''), height_m, latitude, longitude, COALESCE(notes,''), created_at, updated_at`,
		in.SiteID, in.Name, nilIfEmpty(in.Code), in.HeightM, in.Latitude, in.Longitude, nilIfEmpty(in.Notes),
	).Scan(&t.ID, &siteID, &t.Name, &t.Code, &height, &t.Latitude, &t.Longitude, &t.Notes, &t.CreatedAt, &t.UpdatedAt)
	t.SiteID = siteID
	t.HeightM = height
	return &t, err
}

// -------- Links ------------------------------------------------

func (r *Repository) ListLinks(ctx context.Context) ([]Link, error) {
	rows, err := r.P.Query(ctx, `SELECT id, name, topology, master_device_id::text, frequency_mhz, channel_width_mhz, risk, last_checked_at, created_at, updated_at FROM links ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Link, 0)
	for rows.Next() {
		var l Link
		var lastChecked *time.Time
		var freq, width *int
		if err := rows.Scan(&l.ID, &l.Name, &l.Topology, &l.MasterDeviceID, &freq, &width, &l.Risk, &lastChecked, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		l.FrequencyMHz = freq
		l.ChannelWidthMHz = width
		l.LastCheckedAt = lastChecked
		out = append(out, l)
	}
	return out, rows.Err()
}

type CreateLinkInput struct {
	Name            string
	Topology        string
	MasterDeviceID  string
	FrequencyMHz    *int
	ChannelWidthMHz *int
}

func (r *Repository) CreateLink(ctx context.Context, in CreateLinkInput) (*Link, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, &ErrValidation{Msg: "name is required"}
	}
	if !ValidateTopology(in.Topology) {
		return nil, &ErrValidation{Msg: "topology must be ptp|ptmp"}
	}
	if in.MasterDeviceID == "" {
		return nil, &ErrValidation{Msg: "master_device_id is required"}
	}
	var l Link
	var lastChecked *time.Time
	var freq, width *int
	err := r.P.QueryRow(ctx, `
INSERT INTO links(name, topology, master_device_id, frequency_mhz, channel_width_mhz)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, name, topology, master_device_id::text, frequency_mhz, channel_width_mhz, risk, last_checked_at, created_at, updated_at`,
		in.Name, strings.ToLower(in.Topology), in.MasterDeviceID, in.FrequencyMHz, in.ChannelWidthMHz,
	).Scan(&l.ID, &l.Name, &l.Topology, &l.MasterDeviceID, &freq, &width, &l.Risk, &lastChecked, &l.CreatedAt, &l.UpdatedAt)
	l.FrequencyMHz = freq
	l.ChannelWidthMHz = width
	l.LastCheckedAt = lastChecked
	return &l, err
}

// -------- Customers --------------------------------------------

func (r *Repository) ListCustomers(ctx context.Context) ([]Customer, error) {
	rows, err := r.P.Query(ctx, `SELECT id, COALESCE(external_code,''), full_name, COALESCE(phone,''), COALESCE(address,''), site_id::text, tower_id::text, contracted_mbps, status, created_at, updated_at FROM customers ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Customer, 0)
	for rows.Next() {
		var c Customer
		var siteID, towerID *string
		var mbps *int
		if err := rows.Scan(&c.ID, &c.ExternalCode, &c.FullName, &c.Phone, &c.Address, &siteID, &towerID, &mbps, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.SiteID = siteID
		c.TowerID = towerID
		c.ContractedMbps = mbps
		out = append(out, c)
	}
	return out, rows.Err()
}

type CreateCustomerInput struct {
	ExternalCode   string
	FullName       string
	Phone          string
	Address        string
	SiteID         *string
	TowerID        *string
	ContractedMbps *int
}

func (r *Repository) CreateCustomer(ctx context.Context, in CreateCustomerInput) (*Customer, error) {
	if strings.TrimSpace(in.FullName) == "" {
		return nil, &ErrValidation{Msg: "full_name is required"}
	}
	var c Customer
	var siteID, towerID *string
	var mbps *int
	err := r.P.QueryRow(ctx, `
INSERT INTO customers(external_code, full_name, phone, address, site_id, tower_id, contracted_mbps)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, COALESCE(external_code,''), full_name, COALESCE(phone,''), COALESCE(address,''), site_id::text, tower_id::text, contracted_mbps, status, created_at, updated_at`,
		nilIfEmpty(in.ExternalCode), in.FullName, nilIfEmpty(in.Phone), nilIfEmpty(in.Address),
		in.SiteID, in.TowerID, in.ContractedMbps,
	).Scan(&c.ID, &c.ExternalCode, &c.FullName, &c.Phone, &c.Address, &siteID, &towerID, &mbps, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	c.SiteID = siteID
	c.TowerID = towerID
	c.ContractedMbps = mbps
	return &c, err
}

// -------- helpers ----------------------------------------------

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	// simple decimal printer; enough for arg index up to 99
	a := n / 10
	b := n % 10
	return string(rune('0'+a)) + string(rune('0'+b))
}
