package reports

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestProblemCustomersCSV(t *testing.T) {
	now := time.Date(2026, 4, 27, 9, 30, 0, 0, time.UTC)
	apID := "ap-1"
	apName := "AP North"
	towID := "tower-1"
	towName := "Kule Kuzey"
	rows := []ProblemCustomerRow{
		{
			CustomerID:        "c1",
			CustomerName:      "Demo Müşteri 1",
			APDeviceID:        &apID,
			APDeviceName:      &apName,
			TowerID:           &towID,
			TowerName:         &towName,
			Score:             37,
			Severity:          "critical",
			Diagnosis:         "weak_customer_signal",
			RecommendedAction: "schedule_field_visit",
			IsStale:           false,
			CalculatedAt:      now,
		},
	}
	var buf bytes.Buffer
	if err := ProblemCustomersCSV(&buf, rows); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Müşteri ID", "Müşteri Adı", "Skor", "Hesaplandı (UTC)",
		"c1", "Demo Müşteri 1", "37", "critical", "weak_customer_signal",
		"AP North", "Kule Kuzey", "hayır",
		now.Format(time.RFC3339),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("CSV missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestWorkOrdersCSVOverdueFlag(t *testing.T) {
	past := time.Now().Add(-2 * time.Hour)
	rows := []WorkOrderRow{
		{
			ID:                "wo1",
			Title:             "Saha kontrol",
			Diagnosis:         "weak_customer_signal",
			RecommendedAction: "schedule_field_visit",
			Severity:          "critical",
			Status:            "open",
			Priority:          "high",
			ETAAt:             &past,
			OverdueETA:        true,
			CreatedAt:         time.Now(),
		},
	}
	var buf bytes.Buffer
	if err := WorkOrdersCSV(&buf, rows); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "evet") {
		t.Errorf("expected overdue=evet in CSV, got: %s", out)
	}
}

func TestAPHealthCSVHeaders(t *testing.T) {
	var buf bytes.Buffer
	if err := APHealthCSV(&buf, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "AP Cihaz ID,AP Cihaz Adı,Kule ID") {
		t.Errorf("unexpected header: %q", buf.String())
	}
}
