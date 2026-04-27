// Package inventory, devices/sites/towers/links/customers için
// repository ve domain tipleri sağlar.
package inventory

import (
	"errors"
	"net"
	"strings"
	"time"
)

// ErrNotFound, bilinmeyen bir kayıt için döner.
var ErrNotFound = errors.New("not found")

// ErrValidation, kullanıcı girdisi tutarsızsa döner.
type ErrValidation struct{ Msg string }

func (e *ErrValidation) Error() string { return "validation: " + e.Msg }

// validIP reports whether s is a valid IPv4 or IPv6 literal.
func ValidIP(s string) bool {
	if s == "" {
		return true
	}
	return net.ParseIP(s) != nil
}

// Site represents a POP / location.
type Site struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code,omitempty"`
	Region    string    `json:"region,omitempty"`
	Address   string    `json:"address,omitempty"`
	Latitude  *float64  `json:"latitude,omitempty"`
	Longitude *float64  `json:"longitude,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Tower represents a physical tower under a site.
type Tower struct {
	ID        string    `json:"id"`
	SiteID    *string   `json:"site_id,omitempty"`
	Name      string    `json:"name"`
	Code      string    `json:"code,omitempty"`
	HeightM   *float64  `json:"height_m,omitempty"`
	Latitude  *float64  `json:"latitude,omitempty"`
	Longitude *float64  `json:"longitude,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Device represents an inventoried hardware device.
type Device struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Vendor          string    `json:"vendor"`
	Role            string    `json:"role"`
	IPAddress       string    `json:"ip_address,omitempty"`
	SiteID          *string   `json:"site_id,omitempty"`
	TowerID         *string   `json:"tower_id,omitempty"`
	Model           string    `json:"model,omitempty"`
	OSVersion       string    `json:"os_version,omitempty"`
	FirmwareVersion string    `json:"firmware_version,omitempty"`
	Status          string    `json:"status"`
	Tags            []string  `json:"tags"`
	Notes           string    `json:"notes,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Link represents a PTP/PTMP wireless link.
type Link struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Topology        string     `json:"topology"`
	MasterDeviceID  string     `json:"master_device_id"`
	FrequencyMHz    *int       `json:"frequency_mhz,omitempty"`
	ChannelWidthMHz *int       `json:"channel_width_mhz,omitempty"`
	Risk            string     `json:"risk"`
	LastCheckedAt   *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Customer represents a subscriber.
type Customer struct {
	ID             string    `json:"id"`
	ExternalCode   string    `json:"external_code,omitempty"`
	FullName       string    `json:"full_name"`
	Phone          string    `json:"phone,omitempty"`
	Address        string    `json:"address,omitempty"`
	SiteID         *string   `json:"site_id,omitempty"`
	TowerID        *string   `json:"tower_id,omitempty"`
	ContractedMbps *int      `json:"contracted_mbps,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Vendor / Role / Status validators ----------------------------

var validVendors = map[string]struct{}{"mikrotik": {}, "mimosa": {}, "other": {}, "unknown": {}}
var validRoles = map[string]struct{}{
	"ap": {}, "cpe": {}, "ptp_master": {}, "ptp_slave": {}, "router": {}, "switch": {},
}
var validDeviceStatus = map[string]struct{}{
	"active": {}, "retired": {}, "maintenance": {}, "spare": {},
}
var validTopology = map[string]struct{}{"ptp": {}, "ptmp": {}}
var validRisk = map[string]struct{}{"healthy": {}, "watch": {}, "warning": {}, "critical": {}}
var validCustomerStatus = map[string]struct{}{"active": {}, "suspended": {}, "cancelled": {}}

func ValidateVendor(v string) bool       { _, ok := validVendors[strings.ToLower(v)]; return ok }
func ValidateRole(r string) bool         { _, ok := validRoles[strings.ToLower(r)]; return ok }
func ValidateDeviceStatus(s string) bool { _, ok := validDeviceStatus[strings.ToLower(s)]; return ok }
func ValidateTopology(t string) bool     { _, ok := validTopology[strings.ToLower(t)]; return ok }
func ValidateRisk(r string) bool         { _, ok := validRisk[strings.ToLower(r)]; return ok }
func ValidateCustomerStatus(s string) bool {
	_, ok := validCustomerStatus[strings.ToLower(s)]
	return ok
}
