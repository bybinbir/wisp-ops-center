package reports

import (
	"encoding/csv"
	"io"
	"strconv"
	"time"
)

// CSV başlık çevirileri Türkçe; UTF-8 BOM yazmıyoruz çünkü RFC 4180
// uyumsuz; kullanıcı tarafında Excel için BOM gerekirse opsiyonel hale
// getirilebilir. Hepsi text/csv; charset=utf-8 olarak servis edilir.

// WriteCSV, generic bir helper'dır. Her satır için sıralı string slice'a
// dönüşüm yapan callback ile çalışır.
func WriteCSV(w io.Writer, headers []string, rows int, fill func(i int) []string) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write(headers); err != nil {
		return err
	}
	for i := 0; i < rows; i++ {
		if err := cw.Write(fill(i)); err != nil {
			return err
		}
		// Periyodik flush — büyük datasetlerde TCP penceresini kapatmamak için.
		if i%200 == 0 {
			cw.Flush()
			if err := cw.Error(); err != nil {
				return err
			}
		}
	}
	return cw.Error()
}

// ProblemCustomersCSV, problem-customers CSV'sini yazar.
func ProblemCustomersCSV(w io.Writer, rows []ProblemCustomerRow) error {
	headers := []string{
		"Müşteri ID", "Müşteri Adı", "Kule ID", "Kule Adı",
		"AP Cihaz ID", "AP Cihaz Adı",
		"Skor", "Severity", "Tanı", "Önerilen Aksiyon",
		"Bayat?", "Hesaplandı (UTC)",
	}
	return WriteCSV(w, headers, len(rows), func(i int) []string {
		r := rows[i]
		return []string{
			r.CustomerID, r.CustomerName,
			ptrStr(r.TowerID), ptrStr(r.TowerName),
			ptrStr(r.APDeviceID), ptrStr(r.APDeviceName),
			strconv.Itoa(r.Score), r.Severity, r.Diagnosis, r.RecommendedAction,
			boolStr(r.IsStale), r.CalculatedAt.UTC().Format(time.RFC3339),
		}
	})
}

// APHealthCSV, ap-health CSV'sini yazar.
func APHealthCSV(w io.Writer, rows []APHealthRow) error {
	headers := []string{
		"AP Cihaz ID", "AP Cihaz Adı", "Kule ID", "Kule Adı",
		"AP Skor", "Severity", "Toplam Müşteri",
		"Kritik Müşteri", "Uyarı Müşteri", "Sağlıklı Müşteri",
		"Degradation Oranı", "AP Geneli Parazit", "Hesaplandı (UTC)",
	}
	return WriteCSV(w, headers, len(rows), func(i int) []string {
		r := rows[i]
		return []string{
			r.APDeviceID, r.APDeviceName, ptrStr(r.TowerID), ptrStr(r.TowerName),
			strconv.Itoa(r.APScore), r.Severity, strconv.Itoa(r.TotalCustomers),
			strconv.Itoa(r.CriticalCustomers), strconv.Itoa(r.WarningCustomers),
			strconv.Itoa(r.HealthyCustomers),
			strconv.FormatFloat(r.DegradationRatio, 'f', 4, 64),
			boolStr(r.APWideInterference),
			r.CalculatedAt.UTC().Format(time.RFC3339),
		}
	})
}

// TowerRiskCSV, tower-risk CSV'sini yazar.
func TowerRiskCSV(w io.Writer, rows []TowerRiskRow) error {
	headers := []string{
		"Kule ID", "Kule Adı", "Risk Skoru", "Severity", "Hesaplandı (UTC)",
	}
	return WriteCSV(w, headers, len(rows), func(i int) []string {
		r := rows[i]
		return []string{
			r.TowerID, r.TowerName, strconv.Itoa(r.RiskScore),
			r.Severity, r.CalculatedAt.UTC().Format(time.RFC3339),
		}
	})
}

// WorkOrdersCSV, work-orders CSV'sini yazar.
func WorkOrdersCSV(w io.Writer, rows []WorkOrderRow) error {
	headers := []string{
		"İş Emri ID", "Başlık",
		"Müşteri ID", "Müşteri Adı",
		"AP ID", "AP Adı",
		"Kule ID", "Kule Adı",
		"Tanı", "Önerilen Aksiyon",
		"Severity", "Status", "Priority",
		"Atanan", "ETA (UTC)", "ETA Geçti?",
		"Oluşturuldu (UTC)", "Çözüldü (UTC)",
	}
	return WriteCSV(w, headers, len(rows), func(i int) []string {
		r := rows[i]
		return []string{
			r.ID, r.Title,
			ptrStr(r.CustomerID), ptrStr(r.CustomerName),
			ptrStr(r.APDeviceID), ptrStr(r.APDeviceName),
			ptrStr(r.TowerID), ptrStr(r.TowerName),
			r.Diagnosis, r.RecommendedAction,
			r.Severity, r.Status, r.Priority,
			ptrStr(r.AssignedTo), ptrTime(r.ETAAt), boolStr(r.OverdueETA),
			r.CreatedAt.UTC().Format(time.RFC3339), ptrTime(r.ResolvedAt),
		}
	})
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrTime(p *time.Time) string {
	if p == nil {
		return ""
	}
	return p.UTC().Format(time.RFC3339)
}

func boolStr(b bool) string {
	if b {
		return "evet"
	}
	return "hayır"
}
