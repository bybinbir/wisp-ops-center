"use client";
import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  api,
  ApiError,
  WorkOrder,
  WorkOrderStatus,
  WorkOrderPriority,
  WORK_ORDER_STATUSES,
  WORK_ORDER_PRIORITIES,
  STATUS_LABELS,
  PRIORITY_LABELS,
  Severity,
  DIAGNOSIS_LABELS,
  Diagnosis,
} from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextInput } from "@/components/Field";

const SEVERITY_BADGE: Record<Severity, { label: string; bg: string }> = {
  critical: { label: "KRİTİK", bg: "#7d1d1d" },
  warning: { label: "UYARI", bg: "#7a5a00" },
  healthy: { label: "Sağlıklı", bg: "#1c4f1c" },
  unknown: { label: "Bilinmiyor", bg: "#404040" },
};

const PRIORITY_BG: Record<WorkOrderPriority, string> = {
  urgent: "#7d1d1d",
  high: "#a04500",
  medium: "#5a5a5a",
  low: "#2f6e2f",
};

export function WorkOrdersClient() {
  const [rows, setRows] = useState<WorkOrder[] | null>(null);
  const [total, setTotal] = useState<number>(0);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState({
    status: "",
    priority: "",
    severity: "",
    tower_id: "",
    ap_device_id: "",
    assigned_to: "",
  });

  const reload = useCallback(async () => {
    setError(null);
    try {
      const params = new URLSearchParams();
      Object.entries(filters).forEach(([k, v]) => {
        if (v) params.set(k, v);
      });
      params.set("limit", "200");
      const r = await api.get<{ data: WorkOrder[]; total: number }>(
        `/api/v1/work-orders?${params.toString()}`
      );
      setRows(r.data ?? []);
      setTotal(r.total ?? 0);
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 503
          ? "Veritabanı bağlı değil. İş emri tablosu erişilemiyor."
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

  function csvUrl() {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([k, v]) => {
      if (v) params.set(k, v);
    });
    return `/api/v1/reports/work-orders.csv?${params.toString()}`;
  }
  function pdfUrl() {
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([k, v]) => {
      if (v) params.set(k, v);
    });
    return `/api/v1/reports/work-orders.pdf?${params.toString()}`;
  }

  return (
    <div>
      <Toolbar>
        <Field label="Status">
          <Select
            value={filters.status}
            onChange={(e) => setFilters({ ...filters, status: e.target.value })}
          >
            <option value="">Tümü</option>
            {WORK_ORDER_STATUSES.map((s) => (
              <option key={s} value={s}>
                {STATUS_LABELS[s]}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Priority">
          <Select
            value={filters.priority}
            onChange={(e) =>
              setFilters({ ...filters, priority: e.target.value })
            }
          >
            <option value="">Tümü</option>
            {WORK_ORDER_PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {PRIORITY_LABELS[p]}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Severity">
          <Select
            value={filters.severity}
            onChange={(e) =>
              setFilters({ ...filters, severity: e.target.value })
            }
          >
            <option value="">Tümü</option>
            <option value="critical">Yalnız critical</option>
            <option value="warning">Yalnız warning</option>
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
        <Field label="Atanan">
          <TextInput
            placeholder="kullanıcı/ekip"
            value={filters.assigned_to}
            onChange={(e) =>
              setFilters({ ...filters, assigned_to: e.target.value })
            }
          />
        </Field>
        <div style={{ alignSelf: "end", display: "flex", gap: 6 }}>
          <Button onClick={reload}>Yenile</Button>
          <a href={csvUrl()} target="_blank" rel="noreferrer">
            <Button variant="secondary">CSV</Button>
          </a>
          <a href={pdfUrl()} target="_blank" rel="noreferrer">
            <Button variant="secondary">PDF</Button>
          </a>
        </div>
      </Toolbar>

      {error ? <div className="banner">{error}</div> : null}

      <p style={{ color: "var(--text-dim)", fontSize: 12 }}>
        Toplam: <strong>{total}</strong>
      </p>

      <table>
        <thead>
          <tr>
            <th>Başlık</th>
            <th>Severity</th>
            <th>Priority</th>
            <th>Status</th>
            <th>Tanı</th>
            <th>Atanan</th>
            <th>ETA</th>
            <th>Oluştu</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {rows && rows.length === 0 ? (
            <tr>
              <td colSpan={9} className="empty">
                Filtreye uyan iş emri yok.
              </td>
            </tr>
          ) : null}
          {(rows ?? []).map((w) => {
            const sev = SEVERITY_BADGE[w.severity] ?? SEVERITY_BADGE.unknown;
            const overdue =
              w.eta_at &&
              new Date(w.eta_at) < new Date() &&
              w.status !== "resolved" &&
              w.status !== "cancelled";
            return (
              <tr key={w.id}>
                <td>
                  <Link
                    href={`/is-emirleri/${w.id}`}
                    style={{ color: "var(--accent)", fontWeight: 600 }}
                  >
                    {w.title || w.diagnosis}
                  </Link>
                </td>
                <td>
                  <span
                    style={{
                      background: sev.bg,
                      color: "#fff",
                      padding: "2px 6px",
                      borderRadius: 4,
                      fontSize: 11,
                      fontWeight: 700,
                    }}
                  >
                    {sev.label}
                  </span>
                </td>
                <td>
                  <span
                    style={{
                      background: PRIORITY_BG[w.priority],
                      color: "#fff",
                      padding: "2px 6px",
                      borderRadius: 4,
                      fontSize: 11,
                      fontWeight: 700,
                    }}
                  >
                    {PRIORITY_LABELS[w.priority]}
                  </span>
                </td>
                <td>{STATUS_LABELS[w.status]}</td>
                <td>
                  {DIAGNOSIS_LABELS[w.diagnosis as Diagnosis] ?? w.diagnosis}
                </td>
                <td>{w.assigned_to ?? "—"}</td>
                <td>
                  {w.eta_at ? (
                    <span style={{ color: overdue ? "#d54040" : undefined }}>
                      {new Date(w.eta_at).toLocaleString("tr-TR")}
                      {overdue ? " (geçti)" : ""}
                    </span>
                  ) : (
                    "—"
                  )}
                </td>
                <td>{new Date(w.created_at).toLocaleString("tr-TR")}</td>
                <td>
                  <Link href={`/is-emirleri/${w.id}`}>
                    <Button variant="secondary">Detay</Button>
                  </Link>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
