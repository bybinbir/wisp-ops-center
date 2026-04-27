"use client";
import { useEffect, useState, FormEvent } from "react";
import { api, ApiError } from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextInput } from "@/components/Field";
import { Modal } from "@/components/Modal";

type ScheduledCheck = {
  id: string;
  name: string;
  job_type: string;
  schedule_type: string;
  cron_expression?: string;
  timezone: string;
  scope_type: string;
  scope_id?: string;
  action_mode: string;
  risk_level: string;
  enabled: boolean;
  next_run_at?: string;
  last_run_at?: string;
  max_duration_seconds: number;
  max_parallel: number;
  maintenance_window_id?: string;
};

const JOB_TYPES = [
  "mikrotik_readonly_poll",
  "mimosa_readonly_poll",
  "tower_health_check",
  "customer_signal_check",
  "daily_network_check",
  "weekly_network_report",
  "ap_client_ping_latency",
  "ap_client_packet_loss",
  "ap_client_jitter",
  "ap_client_traceroute"
] as const;

const SCHED_TYPES = ["manual", "daily", "weekly", "monthly", "interval"] as const;
const SCOPE_TYPES = ["all_network", "site", "tower", "device", "customer", "link"] as const;
const RISKS = ["low", "medium", "high"] as const;
const MODES = ["report_only", "recommend_only", "manual_approval"] as const;

const empty = {
  name: "",
  job_type: "mikrotik_readonly_poll",
  schedule_type: "daily",
  cron_expression: "0 3",
  timezone: "Europe/Istanbul",
  scope_type: "all_network",
  scope_id: "",
  action_mode: "report_only",
  risk_level: "low",
  enabled: true,
  max_duration_seconds: 60,
  max_parallel: 4
};

export function ScheduledChecksClient() {
  const [list, setList] = useState<ScheduledCheck[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ ...empty });

  async function reload() {
    try {
      const r = await api.get<{ data: ScheduledCheck[] }>("/api/v1/scheduled-checks");
      setList(r.data ?? []);
      setError(null);
    } catch (e) {
      const msg = e instanceof ApiError && e.status === 503
        ? "Veritabanı bağlı değil."
        : e instanceof Error ? e.message : "Bilinmeyen hata";
      setError(msg);
      setList([]);
    }
  }
  useEffect(() => { reload(); }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      await api.post("/api/v1/scheduled-checks", form);
      setOpen(false);
      setForm({ ...empty });
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "İşlem başarısız");
    }
  }

  async function runNow(id: string) {
    try {
      await api.post(`/api/v1/scheduled-checks/${id}/run-now`);
      alert("Run-now kuyruklandı.");
    } catch (e) {
      alert(e instanceof Error ? e.message : "Run-now başarısız");
    }
  }

  async function remove(id: string) {
    if (!confirm("Kontrol silinsin mi?")) return;
    try {
      await api.del(`/api/v1/scheduled-checks/${id}`);
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Silinemedi");
    }
  }

  return (
    <div>
      <div className="banner">
        <strong>Faz 5:</strong> Scheduler aktif. <em>controlled_apply</em>{" "}
        Faz 9'a kadar reddedilir. AP→Client testler düşük riskli ping/loss/jitter/traceroute
        sınırına bağlı; yüksek riskli mikrotik_bandwidth_test ve limited_throughput
        kapalıdır.
      </div>
      <Toolbar>
        <Button onClick={() => setOpen(true)}>+ Yeni Kontrol</Button>
        <Button variant="secondary" onClick={reload}>Yenile</Button>
      </Toolbar>
      {error ? <div className="banner">{error}</div> : null}

      <table>
        <thead>
          <tr>
            <th>Ad</th>
            <th>İş tipi</th>
            <th>Cadence</th>
            <th>Risk</th>
            <th>Aktif</th>
            <th>Sonraki</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {list && list.length === 0 ? (
            <tr><td colSpan={7} className="empty">Tanımlı kontrol yok.</td></tr>
          ) : null}
          {(list ?? []).map((c) => (
            <tr key={c.id}>
              <td>{c.name}</td>
              <td>{c.job_type}</td>
              <td>{c.schedule_type} {c.cron_expression ?? ""}</td>
              <td>
                <span className={`badge ${c.risk_level === "low" ? "good" : c.risk_level === "medium" ? "warn" : "bad"}`}>
                  {c.risk_level}
                </span>
              </td>
              <td><span className={`badge ${c.enabled ? "good" : "warn"}`}>{c.enabled ? "yes" : "no"}</span></td>
              <td>{c.next_run_at ? new Date(c.next_run_at).toLocaleString("tr-TR") : "—"}</td>
              <td style={{ whiteSpace: "nowrap" }}>
                <Button onClick={() => runNow(c.id)}>Run now</Button>{" "}
                <Button variant="danger" onClick={() => remove(c.id)}>Sil</Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <Modal open={open} onClose={() => setOpen(false)} title="Yeni Planlı Kontrol">
        <form onSubmit={submit} style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
          <Field label="Ad *">
            <TextInput required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </Field>
          <Field label="İş tipi">
            <Select value={form.job_type} onChange={(e) => setForm({ ...form, job_type: e.target.value })}>
              {JOB_TYPES.map((j) => <option key={j} value={j}>{j}</option>)}
            </Select>
          </Field>
          <Field label="Cadence">
            <Select value={form.schedule_type} onChange={(e) => setForm({ ...form, schedule_type: e.target.value })}>
              {SCHED_TYPES.map((s) => <option key={s} value={s}>{s}</option>)}
            </Select>
          </Field>
          <Field label="Cron (M H [DoW|DoM])">
            <TextInput value={form.cron_expression} onChange={(e) => setForm({ ...form, cron_expression: e.target.value })} />
          </Field>
          <Field label="Timezone">
            <TextInput value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} />
          </Field>
          <Field label="Scope">
            <Select value={form.scope_type} onChange={(e) => setForm({ ...form, scope_type: e.target.value })}>
              {SCOPE_TYPES.map((s) => <option key={s} value={s}>{s}</option>)}
            </Select>
          </Field>
          <Field label="Scope ID (opsiyonel)">
            <TextInput value={form.scope_id} onChange={(e) => setForm({ ...form, scope_id: e.target.value })} />
          </Field>
          <Field label="Risk">
            <Select value={form.risk_level} onChange={(e) => setForm({ ...form, risk_level: e.target.value })}>
              {RISKS.map((r) => <option key={r} value={r}>{r}</option>)}
            </Select>
          </Field>
          <Field label="Aksiyon Modu">
            <Select value={form.action_mode} onChange={(e) => setForm({ ...form, action_mode: e.target.value })}>
              {MODES.map((m) => <option key={m} value={m}>{m}</option>)}
              <option disabled value="controlled_apply">controlled_apply (Faz 9)</option>
            </Select>
          </Field>
          <Field label="Max Süre (sn)">
            <TextInput type="number" value={String(form.max_duration_seconds)}
              onChange={(e) => setForm({ ...form, max_duration_seconds: Number(e.target.value) })} />
          </Field>
          <div style={{ gridColumn: "1 / -1", display: "flex", justifyContent: "flex-end", gap: 8 }}>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>İptal</Button>
            <Button type="submit">Oluştur</Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
