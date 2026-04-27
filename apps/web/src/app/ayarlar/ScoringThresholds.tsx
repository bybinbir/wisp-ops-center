"use client";
import { useEffect, useState } from "react";
import { api, ApiError, ScoringThreshold } from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { TextInput } from "@/components/Field";

const KEY_LABELS: Record<string, string> = {
  rssi_critical_dbm: "RSSI Kritik (dBm)",
  rssi_warning_dbm: "RSSI Uyarı (dBm)",
  snr_critical_db: "SNR Kritik (dB)",
  snr_warning_db: "SNR Uyarı (dB)",
  ccq_critical_percent: "CCQ Kritik (%)",
  ccq_warning_percent: "CCQ Uyarı (%)",
  packet_loss_critical_percent: "Paket Kaybı Kritik (%)",
  packet_loss_warning_percent: "Paket Kaybı Uyarı (%)",
  latency_critical_ms: "Gecikme Kritik (ms)",
  latency_warning_ms: "Gecikme Uyarı (ms)",
  jitter_critical_ms: "Jitter Kritik (ms)",
  jitter_warning_ms: "Jitter Uyarı (ms)",
  stale_data_minutes: "Veri Tazelik Eşiği (dk)",
  ap_degradation_customer_ratio_warning: "AP Degradation Uyarı Oranı",
  ap_degradation_customer_ratio_critical: "AP Degradation Kritik Oranı",
  severity_healthy_at: "Severity → Healthy ≥",
  severity_warning_at: "Severity → Warning ≥",
};

export function ScoringThresholdsClient() {
  const [rows, setRows] = useState<ScoringThreshold[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  async function reload() {
    setError(null);
    try {
      const r = await api.get<{ data: ScoringThreshold[] }>(
        "/api/v1/scoring-thresholds"
      );
      setRows(r.data ?? []);
      setPending({});
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 503
          ? "Veritabanı bağlı değil — eşikler düzenlenemez."
          : e instanceof Error
            ? e.message
            : "Bilinmeyen hata";
      setError(msg);
      setRows([]);
    }
  }

  useEffect(() => {
    reload();
  }, []);

  function setVal(key: string, v: string) {
    setPending({ ...pending, [key]: v });
  }

  async function save() {
    const updates: Record<string, number> = {};
    for (const [k, v] of Object.entries(pending)) {
      const n = Number(v);
      if (Number.isFinite(n)) updates[k] = n;
    }
    if (Object.keys(updates).length === 0) {
      alert("Değişiklik yok.");
      return;
    }
    setBusy(true);
    try {
      await api.patch("/api/v1/scoring-thresholds", { updates });
      await reload();
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 400
          ? "Geçersiz eşik değeri (bilinmeyen anahtar veya aralık dışı)."
          : e instanceof Error
            ? e.message
            : "Eşik güncellenemedi";
      alert(msg);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="section">
      <h3 className="section-title">Skor Eşikleri (Faz 6)</h3>
      <div className="banner">
        Eşik değerleri skor motorunun penalty ve sınıflandırma kararlarını
        belirler. Geçerli aralık dışındaki değerler 400 ile reddedilir; her
        başarılı güncelleme audit log'a yazılır.
      </div>
      {error ? <div className="banner">{error}</div> : null}
      <Toolbar>
        <Button onClick={reload} variant="secondary" disabled={busy}>
          Yenile
        </Button>
        <Button onClick={save} disabled={busy || Object.keys(pending).length === 0}>
          {Object.keys(pending).length > 0
            ? `Kaydet (${Object.keys(pending).length})`
            : "Kaydet"}
        </Button>
      </Toolbar>
      <table>
        <thead>
          <tr>
            <th>Anahtar</th>
            <th>Mevcut</th>
            <th>Yeni</th>
            <th>Açıklama</th>
            <th>Güncelleme</th>
          </tr>
        </thead>
        <tbody>
          {rows && rows.length === 0 ? (
            <tr>
              <td colSpan={5} className="empty">
                Eşik kaydı yok. Migration 000006 varsayılanları seed eder.
              </td>
            </tr>
          ) : null}
          {(rows ?? []).map((r) => (
            <tr key={r.key}>
              <td>
                <strong>{KEY_LABELS[r.key] ?? r.key}</strong>
                <div style={{ fontSize: 11, color: "var(--text-dim)" }}>
                  <code>{r.key}</code>
                </div>
              </td>
              <td style={{ fontFamily: "monospace" }}>{r.value}</td>
              <td>
                <TextInput
                  type="number"
                  step="any"
                  placeholder={String(r.value)}
                  value={pending[r.key] ?? ""}
                  onChange={(e) => setVal(r.key, e.target.value)}
                />
              </td>
              <td style={{ fontSize: 12 }}>{r.description ?? "—"}</td>
              <td style={{ fontSize: 11 }}>
                {new Date(r.updated_at).toLocaleString("tr-TR")}
                {r.updated_by ? ` · ${r.updated_by}` : ""}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
