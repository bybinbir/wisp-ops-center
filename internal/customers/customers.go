// Package customers, abone (CPE müşterisi) alan modelini tanımlar.
package customers

import "time"

// Customer, faturalandırılan / hizmet verilen son kullanıcıdır.
type Customer struct {
	ID             string
	ExternalCode   string
	FullName       string
	Phone          string
	Address        string
	SiteID         string
	TowerID        string
	APDeviceID     string // bağlı olduğu AP
	CPEDeviceID    string // varsa kendi CPE cihazı
	ContractedMbps int
	Status         Status
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Status, müşteri hizmet durumudur.
type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusCancelled Status = "cancelled"
)
