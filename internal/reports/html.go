package reports

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"time"
)

// FAZ 7 NOT — Açık teknik borç (TODO Phase 8/9):
//
// Tarayıcı dışı bir PDF üretimi için Go ekosistemindeki seçenekler:
//
//   - jung-kurt/gofpdf  (eski, Unicode/Türkçe için font yüklemek gerekiyor)
//   - signintech/gopdf  (gofpdf benzeri; lisans temiz)
//   - chromedp + headless Chrome (kurulumsuzdur ama Chrome dependency)
//
// Sandbox'ta bu paketleri çekmek mümkün değil; daha önemlisi, bağımlılığın
// gizli yan etkileri (CGO, font dosyaları) kurumsal bir ürün için önemli
// risk taşıyor. O nedenle Faz 7'de PDF endpoint'leri "HTML-printable"
// (window.print() ile tarayıcıdan PDF) olarak servis edilir.
//
// HTTP cevabı:
//   - Content-Type: text/html; charset=utf-8
//   - Content-Disposition: inline (UI iframe içinde + Yazdır akışı)
//
// Gerçek server-side PDF üretimi Faz 8 / 9 backlog'una taşındı:
// docs/PHASE_007_WORK_ORDERS_REPORTS.md "Açık Borçlar" bölümüne yazıldı.

const execSummaryTemplate = `<!DOCTYPE html>
<html lang="tr">
<head>
<meta charset="utf-8" />
<title>Yönetici Özeti — WISP Ops Center</title>
<style>
  @page { size: A4; margin: 18mm; }
  body { font-family: -apple-system, "Segoe UI", "Helvetica Neue", Arial, sans-serif;
         color: #1a1a1a; background: #fff; margin: 0; padding: 24px; }
  h1 { font-size: 22px; margin: 0 0 4px 0; }
  h2 { font-size: 16px; margin: 28px 0 8px 0; padding-bottom: 4px; border-bottom: 1px solid #ddd; }
  .meta { color: #666; font-size: 12px; }
  .grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; margin-top: 12px; }
  .card { border: 1px solid #ddd; border-radius: 8px; padding: 10px 14px; }
  .card .label { font-size: 11px; color: #666; text-transform: uppercase; letter-spacing: 0.5px; }
  .card .value { font-size: 22px; font-weight: 700; margin-top: 4px; }
  table { width: 100%; border-collapse: collapse; font-size: 12px; margin-top: 8px; }
  th, td { border-bottom: 1px solid #eee; padding: 6px 8px; text-align: left; }
  th { background: #fafafa; }
  .badge { display: inline-block; padding: 2px 6px; border-radius: 4px; font-size: 11px; font-weight: 700; color: #fff; }
  .b-critical { background: #7d1d1d; }
  .b-warning  { background: #7a5a00; }
  .b-healthy  { background: #1c4f1c; }
  .b-unknown  { background: #404040; }
  .footer { margin-top: 32px; font-size: 11px; color: #888; border-top: 1px solid #eee; padding-top: 10px; }
  .print-btn { position: fixed; top: 16px; right: 16px; background: #2c5cff; color: #fff;
               border: 0; padding: 8px 14px; border-radius: 6px; cursor: pointer; font-size: 13px; }
  @media print { .print-btn { display: none; } }
</style>
</head>
<body>
<button class="print-btn" onclick="window.print()">Yazdır / PDF olarak Kaydet</button>
<h1>WISP Ops Center — Yönetici Özeti</h1>
<div class="meta">
  Oluşturuldu: {{.GeneratedAt.Format "2006-01-02 15:04:05 MST"}} ·
  Dönem: {{.PeriodStart.Format "2006-01-02"}} → {{.PeriodEnd.Format "2006-01-02"}}
</div>

<h2>Müşteri Sağlığı</h2>
<div class="grid">
  <div class="card"><div class="label">Toplam Aktif Müşteri</div><div class="value">{{.TotalCustomers}}</div></div>
  <div class="card"><div class="label">Kritik</div><div class="value">{{.CriticalCustomers}}</div></div>
  <div class="card"><div class="label">Uyarı</div><div class="value">{{.WarningCustomers}}</div></div>
  <div class="card"><div class="label">Bayat Veri</div><div class="value">{{.StaleCustomers}}</div></div>
  <div class="card"><div class="label">AP Geneli Parazit</div><div class="value">{{.APWideInterAffect}}</div></div>
  <div class="card"><div class="label">Açık İş Emri</div><div class="value">{{.OpenWorkOrders}}</div></div>
  <div class="card"><div class="label">Urgent / High</div><div class="value">{{.UrgentOrHighPrio}}</div></div>
  <div class="card"><div class="label">ETA Geçen</div><div class="value">{{.OverdueETA}}</div></div>
</div>

<h2>En Riskli 10 AP</h2>
<table>
  <thead><tr>
    <th>AP</th><th>Kule</th><th>Skor</th><th>Severity</th>
    <th>Müşteri</th><th>Kritik</th><th>Uyarı</th><th>AP Geneli</th>
  </tr></thead>
  <tbody>
  {{range .Top10RiskyAPs}}
    <tr>
      <td>{{if .APDeviceName}}{{.APDeviceName}}{{else}}{{.APDeviceID}}{{end}}</td>
      <td>{{if .TowerName}}{{deref .TowerName}}{{else}}—{{end}}</td>
      <td>{{.APScore}}</td>
      <td><span class="badge b-{{.Severity}}">{{sevLabel .Severity}}</span></td>
      <td>{{.TotalCustomers}}</td>
      <td>{{.CriticalCustomers}}</td>
      <td>{{.WarningCustomers}}</td>
      <td>{{if .APWideInterference}}Evet{{else}}—{{end}}</td>
    </tr>
  {{else}}
    <tr><td colspan="8">Skor bulunamadı.</td></tr>
  {{end}}
  </tbody>
</table>

<h2>En Riskli 10 Kule</h2>
<table>
  <thead><tr><th>Kule</th><th>Risk Skoru</th><th>Severity</th><th>Hesaplandı</th></tr></thead>
  <tbody>
  {{range .Top10RiskyTowers}}
    <tr>
      <td>{{if .TowerName}}{{.TowerName}}{{else}}{{.TowerID}}{{end}}</td>
      <td>{{.RiskScore}}</td>
      <td><span class="badge b-{{.Severity}}">{{sevLabel .Severity}}</span></td>
      <td>{{.CalculatedAt.Format "2006-01-02 15:04"}}</td>
    </tr>
  {{else}}
    <tr><td colspan="4">Kule risk skoru yok.</td></tr>
  {{end}}
  </tbody>
</table>

<h2>En Sık Tekrar Eden Tanılar</h2>
<table>
  <thead><tr><th>Tanı</th><th>Müşteri Sayısı</th></tr></thead>
  <tbody>
  {{range .Top10Diagnoses}}
    <tr><td>{{diagLabel .Diagnosis}}</td><td>{{.Count}}</td></tr>
  {{else}}
    <tr><td colspan="2">Veri yok.</td></tr>
  {{end}}
  </tbody>
</table>

<h2>Son 7 Gün Trendi</h2>
<table>
  <thead><tr><th>Gün</th><th>Kritik</th><th>Uyarı</th><th>Sağlıklı</th></tr></thead>
  <tbody>
  {{range .Trend7d}}
    <tr><td>{{.Day.Format "2006-01-02"}}</td><td>{{.Critical}}</td><td>{{.Warning}}</td><td>{{.Healthy}}</td></tr>
  {{else}}
    <tr><td colspan="4">Henüz skor üretilmemiş.</td></tr>
  {{end}}
  </tbody>
</table>

<div class="footer">
  Generated by WISP Ops Center · phase=7 · safety: read-only · PDF üretimi tarayıcıdan
  "Yazdır / PDF olarak Kaydet" ile alınır. Server-side PDF rendering Faz 8/9 backlog'unda.
</div>
</body>
</html>`

const workOrdersTemplate = `<!DOCTYPE html>
<html lang="tr">
<head>
<meta charset="utf-8" />
<title>İş Emirleri Raporu — WISP Ops Center</title>
<style>
  @page { size: A4 landscape; margin: 14mm; }
  body { font-family: -apple-system, "Segoe UI", "Helvetica Neue", Arial, sans-serif;
         color: #1a1a1a; background: #fff; margin: 0; padding: 18px; }
  h1 { font-size: 20px; margin: 0 0 4px 0; }
  .meta { color: #666; font-size: 12px; margin-bottom: 12px; }
  table { width: 100%; border-collapse: collapse; font-size: 11px; }
  th, td { border-bottom: 1px solid #eee; padding: 5px 6px; text-align: left; vertical-align: top; }
  th { background: #fafafa; }
  .badge { display: inline-block; padding: 2px 6px; border-radius: 4px; font-size: 10px; font-weight: 700; color: #fff; }
  .b-urgent { background: #7d1d1d; }
  .b-high   { background: #a04500; }
  .b-medium { background: #5a5a5a; }
  .b-low    { background: #2f6e2f; }
  .b-critical { background: #7d1d1d; }
  .b-warning  { background: #7a5a00; }
  .footer { margin-top: 24px; font-size: 11px; color: #888; border-top: 1px solid #eee; padding-top: 10px; }
  .print-btn { position: fixed; top: 16px; right: 16px; background: #2c5cff; color: #fff;
               border: 0; padding: 8px 14px; border-radius: 6px; cursor: pointer; font-size: 13px; }
  @media print { .print-btn { display: none; } }
</style>
</head>
<body>
<button class="print-btn" onclick="window.print()">Yazdır / PDF olarak Kaydet</button>
<h1>İş Emirleri Raporu</h1>
<div class="meta">
  Oluşturuldu: {{.GeneratedAt.Format "2006-01-02 15:04:05 MST"}} ·
  Toplam: {{len .Rows}}
  {{if .Filter.Status}} · status={{.Filter.Status}}{{end}}
  {{if .Filter.Priority}} · priority={{.Filter.Priority}}{{end}}
</div>
<table>
  <thead><tr>
    <th>Başlık</th><th>Müşteri</th><th>AP</th><th>Kule</th>
    <th>Tanı</th><th>Severity</th><th>Priority</th>
    <th>Status</th><th>Atanan</th><th>ETA</th><th>Oluştu</th>
  </tr></thead>
  <tbody>
  {{range .Rows}}
    <tr>
      <td>{{.Title}}</td>
      <td>{{if .CustomerName}}{{deref .CustomerName}}{{else if .CustomerID}}{{deref .CustomerID}}{{else}}—{{end}}</td>
      <td>{{if .APDeviceName}}{{deref .APDeviceName}}{{else}}—{{end}}</td>
      <td>{{if .TowerName}}{{deref .TowerName}}{{else}}—{{end}}</td>
      <td>{{diagLabel .Diagnosis}}</td>
      <td><span class="badge b-{{.Severity}}">{{sevLabel .Severity}}</span></td>
      <td><span class="badge b-{{.Priority}}">{{.Priority}}</span></td>
      <td>{{.Status}}{{if .OverdueETA}} · ETA aşıldı{{end}}</td>
      <td>{{if .AssignedTo}}{{deref .AssignedTo}}{{else}}—{{end}}</td>
      <td>{{if .ETAAt}}{{(.ETAAt).Format "2006-01-02 15:04"}}{{else}}—{{end}}</td>
      <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
    </tr>
  {{else}}
    <tr><td colspan="11">Filtreye uyan iş emri yok.</td></tr>
  {{end}}
  </tbody>
</table>

<div class="footer">
  Generated by WISP Ops Center · phase=7 · safety: read-only · server-side PDF Faz 8/9'a ertelendi.
</div>
</body>
</html>`

// WorkOrdersHTMLContext, work order rapor template'inin kontekstidir.
type WorkOrdersHTMLContext struct {
	GeneratedAt time.Time
	Rows        []WorkOrderRow
	Filter      ReportsFilter
}

var diagLabels = map[string]string{
	"healthy":                      "Sağlıklı",
	"weak_customer_signal":         "Zayıf Müşteri Sinyali",
	"possible_cpe_alignment_issue": "CPE Yönlendirme Sorunu",
	"ap_wide_interference":         "AP Genelinde Parazit",
	"ptp_link_degradation":         "PtP Link Kötüleşmesi",
	"frequency_channel_risk":       "Frekans/Kanal Riski",
	"high_latency":                 "Yüksek Gecikme",
	"packet_loss":                  "Paket Kaybı",
	"unstable_jitter":              "Kararsız Jitter",
	"device_offline":               "Cihaz Çevrimdışı",
	"stale_data":                   "Veri Bayat",
	"data_insufficient":            "Yetersiz Veri",
}

var sevLabels = map[string]string{
	"critical": "KRİTİK",
	"warning":  "UYARI",
	"healthy":  "Sağlıklı",
	"unknown":  "Bilinmiyor",
}

func tplFuncs() template.FuncMap {
	return template.FuncMap{
		"diagLabel": func(d string) string {
			if v, ok := diagLabels[d]; ok {
				return v
			}
			return d
		},
		"sevLabel": func(s string) string {
			if v, ok := sevLabels[s]; ok {
				return v
			}
			return s
		},
		"deref": func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		},
	}
}

// RenderExecutiveSummaryHTML, yönetici özetini HTML olarak yazar.
func RenderExecutiveSummaryHTML(w io.Writer, es ExecutiveSummary) error {
	tpl, err := template.New("exec").Funcs(tplFuncs()).Parse(execSummaryTemplate)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, es); err != nil {
		return fmt.Errorf("render exec summary: %w", err)
	}
	_, err = io.Copy(w, &buf)
	return err
}

// RenderWorkOrdersHTML, iş emri raporunu HTML olarak yazar.
func RenderWorkOrdersHTML(w io.Writer, ctx WorkOrdersHTMLContext) error {
	tpl, err := template.New("wo").Funcs(tplFuncs()).Parse(workOrdersTemplate)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return fmt.Errorf("render wo report: %w", err)
	}
	_, err = io.Copy(w, &buf)
	return err
}
