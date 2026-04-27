"use client";
import { useEffect, useState, useCallback } from "react";
import {
  api,
  ApiError,
  CustomerWithIssue,
  Severity,
  Diagnosis,
  DIAGNOSES,
  DIAGNOSIS_LABELS,
  ACTION_LABELS,
} from "@/lib/api";
import Link from "next/link";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextInput } from "@/components/Field";

const SEVERITY_BADGE: Record<Severity, { label: string; bg: string; fg: string }> = {
  critical: { label: "KRİTİK", bg: "#7d1d1d", fg: "#fff" },
  warning: { label: "UYARI", bg: "#7a5a00", fg: "#fff" },
  healthy: { label: "Sağlıklı", bg: "#1c4f1c", fg: "#fff" },
  unknown: { label: "Bilinmiyor", bg: "#404040", fg: "#fff" },
};

export function ProblemCustomersClient() {
  const [rows, setRows] = useState<CustomerWithIssue[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [filters, setFilters] = useState({
    severity: "",
    diagnosis: "",
    tower_id: "",
    ap_device_id: "",
    stale: false,
  });

  const reload = useCallback(async () => {
    setError(null);
    try {
      const params = new URLSearchParams();
      if (filters.severity) params.set("severity", filters.severity);
      if (filters.diagnosis) params.set("diagnosis", filters.diagnosis);
      if (filters.tower_id) params.set("tower_id", filters.tower_id);
      if (filters.ap_device_id) params.set("ap_device_id", filters.ap_device_id);
      if (filters.stale) params.set("stale", "true");
      const qs = params.toString();
      const r = await api.get<{ data: CustomerWithIssue[] }>(
        `/api/v1/customers-with-issues${qs ? "?" + qs : ""}`
      );
      setRows(r.data ?? []);
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 503
          ? "Veritabanı bağlı değil. Skor motoru çalışmıyor."
          : e instanceof Error
            ? e.message
            : "Bilinmeyen hata";
      setError(msg);
      setRows([]);
    }
  }, [filters]);

  useEffect(() => {
    reload();
  }, [reload]);

  async function recalc(id: string) {
    setBusy(true);
    try {
      await api.post(`/api/v1/customers/${id}/calculate-score`);
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Skor hesaplanamadı");
    } finally {
      setBusy(false);
    }
  }

  async function createCandidate(id: string) {
    setBusy(true);
    try {
      const res = await api.post<{
        data: { duplicate?: boolean; id: string };
      }>(`/api/v1/customers/${id}/create-work-order-from-score`);
      if (res.data.duplicate) {
        alert(
          "Aynı tanı için zaten açık bir iş emri adayı var. Mevcut id: " +
            res.data.id
        );
      } else {
        alert("İş emri adayı oluşturuldu: " + res.data.id);
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Aday oluşturulamadı");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <Toolbar>
        <Field label="Severity">
          <Select
            value={filters.severity}
            onChange={(e) =>
              setFilters({ ...filters, severity: e.target.value })
            }
          >
            <option value="">Tümü (warning + critical)</option>
            <option value="critical">Yalnız critical</option>
            <option value="warning">Yalnız warning</option>
          </Select>
        </Field>
        <Field label="Tanı">
          <Select
            value={filters.diagnosis}
            onChange={(e) =>
              setFilters({ ...filters, diagnosis: e.target.value })
            }
          >
            <option value="">Tümü</option>
            {DIAGNOSES.map((d) => (
              <option key={d} value={d}>
                {DIAGNOSIS_LABELS[d as Diagnosis]}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Tower ID">
          <TextInput
            placeholder="UUID"
            value={filters.tower_id}
            onChange={(e) =>
              setFilters({ ...filters, tower_id: e.target.value })
            }
          />
        </Field>
        <Field label="AP Device ID">
          <TextInput
            placeholder="UUID"
            value={filters.ap_device_id}
            onChange={(e) =>
              setFilters({ ...filters, ap_device_id: e.target.value })
            }
          />
        </Field>
        <Field label="Yalnız bayat">
          <Select
            value={filters.stale ? "true" : "false"}
            onChange={(e) =>
              setFilters({ ...filters, stale: e.target.value === "true" })
            }
          >
            <option value="false">Hayır</option>
            <option value="true">Evet</option>
          </Select>
        </Field>
        <div style={{ alignSelf: "end" }}>
          <Button onClick={reload} disabled={busy}>
            Yenile
          </Button>
        </div>
      </Toolbar>

      {error ? <div className="banner">{error}</div> : null}

      <table>
        <thead>
          <tr>
            <th>Müşteri</th>
            <th>Tower</th>
            <th>AP</th>
            <th>Skor</th>
            <th>Severity</th>
            <th>Tanı</th>
            <th>Önerilen Aksiyon</th>
            <th>Bayat?</th>
            <th>Son Hesap</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {rows && rows.length === 0 ? (
            <tr>
              <td colSpan={10} className="empty">
                Filtreye uyan sorunlu müşteri yok.
              </td>
            </tr>
          ) : null}
          {(rows ?? []).map((r) => {
            const sev = SEVERITY_BADGE[r.severity] ?? SEVERITY_BADGE.unknown;
            return (
              <tr key={r.customer_id}>
                <td>
                  <Link
                    href={`/musteriler/${r.customer_id}`}
                    style={{ color: "var(--accent)" }}
                  >
                    {r.customer_name || r.customer_id.slice(0, 8)}
                  </Link>
                </td>
                <td>
                  {r.tower_id ? (
                    <code style={{ fontSize: 11 }}>
                      {r.tower_id.slice(0, 8)}…
                    </code>
                  ) : (
                    "—"
                  )}
                </td>
                <td>
                  {r.ap_device_id ? (
                    <code style={{ fontSize: 11 }}>
                      {r.ap_device_id.slice(0, 8)}…
                    </code>
                  ) : (
                    "—"
                  )}
                </td>
                <td style={{ fontWeight: 700 }}>{r.score}</td>
                <td>
                  <span
                    style={{
                      background: sev.bg,
                      color: sev.fg,
                      padding: "2px 8px",
                      borderRadius: 4,
                      fontSize: 11,
                      fontWeight: 700,
                    }}
                  >
                    {sev.label}
                  </span>
                </td>
                <td>
                  {DIAGNOSIS_LABELS[r.diagnosis] ?? r.diagnosis}
                </td>
                <td>
                  {ACTION_LABELS[r.recommended_action] ?? r.recommended_action}
                </td>
                <td>{r.is_stale ? "Evet" : "—"}</td>
                <td>
                  {new Date(r.calculated_at).toLocaleString("tr-TR")}
                </td>
                <td style={{ whiteSpace: "nowrap" }}>
                  <Button
                    variant="secondary"
                    onClick={() => recalc(r.customer_id)}
                    disabled={busy}
                  >
                    Skoru Yenile
                  </Button>{" "}
                  <Button
                    onClick={() => createCandidate(r.customer_id)}
                    disabled={busy}
                  >
                    İş Emri Adayı
                  </Button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
