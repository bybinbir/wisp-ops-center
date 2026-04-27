// Package reports, günlük/haftalık operasyon raporlarının veri yapısını
// tanımlar. Faz 1'de yalnızca tip ve oluşturucu iskeletleri vardır.
package reports

import "time"

// Kind, rapor türünü tanımlar.
type Kind string

const (
	KindDaily  Kind = "daily"
	KindWeekly Kind = "weekly"
	KindAdHoc  Kind = "ad_hoc"
)

// Format, raporun çıktı biçimini tanımlar.
type Format string

const (
	FormatJSON Format = "json"
	FormatHTML Format = "html"
	FormatPDF  Format = "pdf"
)

// Report, üretilmiş bir raporun özetidir.
type Report struct {
	ID          string
	Kind        Kind
	GeneratedAt time.Time
	PeriodStart time.Time
	PeriodEnd   time.Time
	Format      Format
	Summary     Summary
	URL         string
}

// Summary, rapor anlık-özet bölümüdür.
type Summary struct {
	TotalDevices       int
	OnlineDevices      int
	OfflineDevices     int
	CriticalCustomers  int
	WatchCustomers     int
	CriticalLinks      int
	OpenWorkOrders     int
	PendingApprovals   int
	RecommendationsNew int
}
