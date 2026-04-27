"use client";
import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  api,
  ApiError,
  CustomerSignalScore,
  WorkOrderCandidate,
  WorkOrder,
  Severity,
  Diagnosis,
  DIAGNOSIS_LABELS,
  ACTION_LABELS,
} from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";

const SEV_COLOR: Record<Severity, string> = {
  critical: "#ff6b6b",
  warning: "#f4b400",
  healthy: "#4caf50",
  unknown: "#888",
};

function fmt(n: number | null | undefined, suffix: string, digits = 1): string {
  if (n === null || n === undefined) return "—";
  return `${n.toFixed(digits)}${suffix}`;
}

export function CustomerDetailClient({ customerId }: { customerId: string }) {
  const [latest, setLatest] = useState<CustomerSignalScore | null>(null);
  const [history, setHistory] = useState<CustomerSignalScore[]>([]);
  const [candidates, setCandidates] = useState<WorkOrderCandidate[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const reload = useCallback(async () => {
    setError(null);
    try {
      const [s, h, c] = await Promise.allSettled([
        api.get<{ data: CustomerSignalScore }>(
          `/api/v1/customers/${customerId}/signal-score`
        ),
        api.get<{ data: CustomerSignalScore[] }>(
          `/api/v1/customers/${customerId}/signal-history?limit=20`
        ),
        api.get<{ data: WorkOrderCandidate[] }>(
          `/api/v1/work-order-candidates?status=open`
        ),
      ]);
      if (s.status === "fulfilled") setLatest(s.value.data);
      else if (s.reason instanceof ApiError && s.reason.status === 404)
        setLatest(null);
      else if (s.reason instanceof ApiError && s.reason.status === 503) {
        setError("Veritabanı bağlı değil.");
        return;
      }
      if (h.status === "fulfilled") setHistory(h.value.data ?? []);
      if (c.status === "fulfilled") {
        setCandidates(
          (c.value.data ?? []).filter((x) => x.customer_id === customerId)
        );
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Bilinmeyen hata");
    }
  }, [customerId]);

  useEffect(() => {
    reload();
  }, [reload]);

  async function recalc() {
    setBusy(true);
    try {
      await api.post(`/api/v1/customers/${customerId}/calculate-score`);
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Skor hesaplanamadı");
    } finally {
      setBusy(false);
    }
  }

  async function createCandidate() {
    setBusy(true);
    try {
      const res = await api.post<{ data: { duplicate?: boolean; id: string } }>(
        `/api/v1/customers/${customerId}/create-work-order-from-score`
      );
      if (res.data.duplicate) {
        alert("Zaten açık aday var: " + res.data.id);
      } else {
        alert("İş emri adayı oluşturuldu.");
      }
      await reload();
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 422
          ? "Sağlıklı/bilinmeyen skor için aday oluşturulamaz."
          : e instanceof Error
            ? e.message
            : "Aday oluşturulamadı";
      alert(msg);
    } finally {
      setBusy(false);
    }
  }

  async function promote(candidateId: string) {
    setBusy(true);
    try {
      const res = await api.post<{ data: WorkOrder; duplicate: boolean }>(
        `/api/v1/work-order-candidates/${candidateId}/promote`,
        {}
      );
      if (res.duplicate) {
        alert(
          "Bu aday zaten iş emrine dönüştürülmüş. Mevcut iş emrine yönlendiriliyor."
        );
      }
      window.location.href = `/is-emirleri/${res.data.id}`;
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 422
          ? "Aday promote edilemez (kapatılmış olabilir)."
          : e instanceof Error
            ? e.message
            : "Promote başarısız";
      alert(msg);
    } finally {
      setBusy(false);
    }
  }

  async function dismiss(candidateId: string) {
    if (!confirm("Adayı dismiss etmek istediğinizden emin misiniz?")) return;
    setBusy(true);
    try {
      await api.patch(`/api/v1/work-order-candidates/${candidateId}`, {
        status: "dismissed",
      });
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Dismiss başarısız");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <Toolbar>
        <Button onClick={recalc} disabled={busy}>
          Skoru Yenile
        </Button>
        <Button variant="secondary" onClick={createCandidate} disabled={busy}>
          İş Emri Adayı Oluştur
        </Button>
        <code style={{ fontSize: 11, color: "var(--text-dim)" }}>
          customer_id: {customerId}
        </code>
      </Toolbar>

      {error ? <div className="banner">{error}</div> : null}

      <section className="section">
        <h3 className="section-title">Son Skor</h3>
        {latest ? (
          <div className="cards">
            <div
              style={{
                padding: 16,
                border: "1px solid var(--border)",
                borderRadius: 8,
                background: "var(--panel-2)",
                minWidth: 200,
              }}
            >
              <div style={{ color: "var(--text-dim)", fontSize: 11 }}>
                KALİTE SKORU
              </div>
              <div
                style={{
                  fontSize: 36,
                  fontWeight: 800,
                  color: SEV_COLOR[latest.severity] ?? "#888",
                }}
              >
                {latest.score}
              </div>
              <div style={{ fontSize: 11, color: "var(--text-dim)" }}>
                {(latest.severity ?? "unknown").toUpperCase()}
                {latest.is_stale ? " · BAYAT" : ""}
              </div>
            </div>
            <div
              style={{
                padding: 16,
                border: "1px solid var(--border)",
                borderRadius: 8,
                background: "var(--panel-2)",
                flex: 1,
              }}
            >
              <div style={{ color: "var(--text-dim)", fontSize: 11 }}>TANI</div>
              <div style={{ fontSize: 16, fontWeight: 600 }}>
                {DIAGNOSIS_LABELS[latest.diagnosis as Diagnosis] ??
                  latest.diagnosis}
              </div>
              <div style={{ fontSize: 13, marginTop: 6 }}>
                <strong>Önerilen aksiyon:</strong>{" "}
                {ACTION_LABELS[latest.recommended_action] ??
                  latest.recommended_action}
              </div>
              <div style={{ fontSize: 11, color: "var(--text-dim)", marginTop: 6 }}>
                Hesaplanma:{" "}
                {new Date(latest.calculated_at).toLocaleString("tr-TR")}
              </div>
            </div>
          </div>
        ) : (
          <div className="empty">
            Bu müşteri için henüz skor üretilmemiş. <em>Skoru Yenile</em>{" "}
            ile yeni hesaplama tetikleyin.
          </div>
        )}
      </section>

      {latest ? (
        <section className="section">
          <h3 className="section-title">Metrikler</h3>
          <table>
            <tbody>
              <tr>
                <th>RSSI</th>
                <td>{fmt(latest.rssi_dbm, " dBm")}</td>
                <th>SNR</th>
                <td>{fmt(latest.snr_db, " dB")}</td>
              </tr>
              <tr>
                <th>CCQ</th>
                <td>{fmt(latest.ccq, "%")}</td>
                <th>Paket Kaybı</th>
                <td>{fmt(latest.packet_loss_pct, "%")}</td>
              </tr>
              <tr>
                <th>Gecikme (avg)</th>
                <td>{fmt(latest.avg_latency_ms, " ms", 0)}</td>
                <th>Jitter</th>
                <td>{fmt(latest.jitter_ms, " ms", 0)}</td>
              </tr>
              <tr>
                <th>7g Trend</th>
                <td colSpan={3}>{fmt(latest.signal_trend_7d, " dB/gün", 2)}</td>
              </tr>
            </tbody>
          </table>
        </section>
      ) : null}

      {latest && latest.reasons && latest.reasons.length > 0 ? (
        <section className="section">
          <h3 className="section-title">Evidence</h3>
          <ul style={{ fontSize: 13, lineHeight: 1.7 }}>
            {latest.reasons.map((r, i) => (
              <li key={i}>{r}</li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="section">
        <h3 className="section-title">Skor Geçmişi (son 20)</h3>
        {history.length === 0 ? (
          <div className="empty">Geçmiş yok.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Tarih</th>
                <th>Skor</th>
                <th>Severity</th>
                <th>Tanı</th>
                <th>Bayat?</th>
              </tr>
            </thead>
            <tbody>
              {history.map((h) => (
                <tr key={h.id}>
                  <td>{new Date(h.calculated_at).toLocaleString("tr-TR")}</td>
                  <td style={{ fontWeight: 700 }}>{h.score}</td>
                  <td style={{ color: SEV_COLOR[h.severity] }}>{h.severity}</td>
                  <td>
                    {DIAGNOSIS_LABELS[h.diagnosis as Diagnosis] ?? h.diagnosis}
                  </td>
                  <td>{h.is_stale ? "Evet" : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="section">
        <h3 className="section-title">İş Emri Adayları</h3>
        {candidates.length === 0 ? (
          <div className="empty">Açık aday yok.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Tanı</th>
                <th>Severity</th>
                <th>Önerilen Aksiyon</th>
                <th>Status</th>
                <th>Oluşturulma</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {candidates.map((c) => (
                <tr key={c.id}>
                  <td>
                    {DIAGNOSIS_LABELS[c.diagnosis as Diagnosis] ?? c.diagnosis}
                  </td>
                  <td>{c.severity}</td>
                  <td>
                    {ACTION_LABELS[c.recommended_action] ??
                      c.recommended_action}
                  </td>
                  <td>
                    {c.status}
                    {c.promoted_work_order_id ? (
                      <>
                        {" · "}
                        <Link
                          href={`/is-emirleri/${c.promoted_work_order_id}`}
                          style={{ color: "var(--accent)" }}
                        >
                          iş emri →
                        </Link>
                      </>
                    ) : null}
                  </td>
                  <td>{new Date(c.created_at).toLocaleString("tr-TR")}</td>
                  <td style={{ whiteSpace: "nowrap" }}>
                    {c.status === "open" ? (
                      <>
                        <Button
                          onClick={() => promote(c.id)}
                          disabled={busy}
                        >
                          İş Emrine Çevir
                        </Button>{" "}
                        <Button
                          variant="secondary"
                          onClick={() => dismiss(c.id)}
                          disabled={busy}
                        >
                          Dismiss
                        </Button>
                      </>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
