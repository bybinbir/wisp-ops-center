"use client";
import { useEffect, useState, useCallback } from "react";
import {
  api,
  ApiError,
  WorkOrder,
  WorkOrderEvent,
  WorkOrderStatus,
  WorkOrderPriority,
  WORK_ORDER_PRIORITIES,
  STATUS_LABELS,
  PRIORITY_LABELS,
  DIAGNOSIS_LABELS,
  ACTION_LABELS,
  Diagnosis,
} from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextArea, TextInput } from "@/components/Field";

type Props = { id: string };

export function WorkOrderDetailClient({ id }: Props) {
  const [wo, setWo] = useState<WorkOrder | null>(null);
  const [events, setEvents] = useState<WorkOrderEvent[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [assignee, setAssignee] = useState<string>("");
  const [note, setNote] = useState<string>("");
  const [priority, setPriority] = useState<WorkOrderPriority | "">("");
  const [eta, setEta] = useState<string>("");

  const load = useCallback(async () => {
    setError(null);
    try {
      const r = await api.get<{
        data: WorkOrder;
        events: WorkOrderEvent[] | null;
      }>(`/api/v1/work-orders/${id}`);
      setWo(r.data);
      setEvents(r.events ?? []);
      setAssignee(r.data.assigned_to ?? "");
      setPriority(r.data.priority);
      setEta(r.data.eta_at ? r.data.eta_at.slice(0, 16) : "");
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 404
          ? "İş emri bulunamadı."
          : e instanceof ApiError && e.status === 503
            ? "Veritabanı bağlı değil."
            : e instanceof Error
              ? e.message
              : "Bilinmeyen hata";
      setError(msg);
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  async function patch(body: Record<string, unknown>) {
    setBusy(true);
    try {
      await api.patch(`/api/v1/work-orders/${id}`, body);
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Güncelleme başarısız");
    } finally {
      setBusy(false);
    }
  }

  async function transition(target: WorkOrderStatus) {
    await patch({ status: target, note: note || undefined });
    setNote("");
  }

  async function saveAssignee() {
    setBusy(true);
    try {
      await api.post(`/api/v1/work-orders/${id}/assign`, {
        assigned_to: assignee,
        note: note || undefined,
        auto_start: true,
      });
      setNote("");
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Atama başarısız");
    } finally {
      setBusy(false);
    }
  }

  async function resolve() {
    setBusy(true);
    try {
      await api.post(`/api/v1/work-orders/${id}/resolve`, {
        note: note || undefined,
      });
      setNote("");
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Çözme başarısız");
    } finally {
      setBusy(false);
    }
  }

  async function cancelWO() {
    if (!confirm("İş emrini iptal etmek istediğinizden emin misiniz?")) return;
    setBusy(true);
    try {
      await api.post(`/api/v1/work-orders/${id}/cancel`, {
        note: note || undefined,
      });
      setNote("");
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "İptal başarısız");
    } finally {
      setBusy(false);
    }
  }

  async function addNote() {
    if (!note.trim()) return;
    setBusy(true);
    try {
      await api.post(`/api/v1/work-orders/${id}/events`, {
        event_type: "note_added",
        note,
      });
      setNote("");
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Not eklenemedi");
    } finally {
      setBusy(false);
    }
  }

  async function updatePriorityOrETA() {
    const body: Record<string, unknown> = {};
    if (priority && wo && priority !== wo.priority) body.priority = priority;
    if (eta) body.eta_at = new Date(eta).toISOString();
    if (Object.keys(body).length === 0) return;
    await patch(body);
  }

  if (error) return <div className="banner">{error}</div>;
  if (!wo)
    return (
      <p style={{ color: "var(--text-dim)" }}>Yükleniyor…</p>
    );

  const overdue =
    wo.eta_at &&
    new Date(wo.eta_at) < new Date() &&
    wo.status !== "resolved" &&
    wo.status !== "cancelled";
  const terminal = wo.status === "resolved" || wo.status === "cancelled";

  return (
    <div>
      <div className="cards" style={{ marginBottom: 12 }}>
        <div className="card">
          <p className="card-title">Status</p>
          <p className="card-value">{STATUS_LABELS[wo.status]}</p>
          <div className="card-meta">
            {wo.resolved_at
              ? "Çözüldü " + new Date(wo.resolved_at).toLocaleString("tr-TR")
              : "Aktif"}
          </div>
        </div>
        <div className="card">
          <p className="card-title">Severity / Priority</p>
          <p className="card-value">
            {wo.severity}
            {" · "}
            {PRIORITY_LABELS[wo.priority]}
          </p>
          <div className="card-meta">{DIAGNOSIS_LABELS[wo.diagnosis as Diagnosis] ?? wo.diagnosis}</div>
        </div>
        <div className="card">
          <p className="card-title">ETA</p>
          <p className="card-value" style={{ color: overdue ? "#d54040" : undefined }}>
            {wo.eta_at ? new Date(wo.eta_at).toLocaleString("tr-TR") : "—"}
          </p>
          <div className="card-meta">{overdue ? "ETA aşıldı" : "Süresinde"}</div>
        </div>
        <div className="card">
          <p className="card-title">Atanan</p>
          <p className="card-value">{wo.assigned_to ?? "—"}</p>
          <div className="card-meta">
            {wo.customer_id
              ? "Müşteri " + wo.customer_id.slice(0, 8) + "…"
              : "—"}
          </div>
        </div>
      </div>

      <section className="section">
        <h3 className="section-title">Detay</h3>
        <p>
          <strong>Başlık:</strong> {wo.title}
        </p>
        <p>
          <strong>Tanı:</strong>{" "}
          {DIAGNOSIS_LABELS[wo.diagnosis as Diagnosis] ?? wo.diagnosis}
        </p>
        <p>
          <strong>Önerilen aksiyon:</strong>{" "}
          {ACTION_LABELS[wo.recommended_action] ?? wo.recommended_action}
        </p>
        {wo.description ? (
          <p>
            <strong>Açıklama:</strong> {wo.description}
          </p>
        ) : null}
        {wo.resolution_note ? (
          <p>
            <strong>Çözüm notu:</strong> {wo.resolution_note}
          </p>
        ) : null}
        {wo.source_candidate_id ? (
          <p style={{ color: "var(--text-dim)", fontSize: 12 }}>
            Kaynak aday: <code>{wo.source_candidate_id}</code>
          </p>
        ) : null}
      </section>

      {!terminal ? (
        <section className="section">
          <h3 className="section-title">Aksiyonlar</h3>
          <Toolbar>
            <Field label="Atanan (kullanıcı / ekip)">
              <TextInput
                value={assignee}
                onChange={(e) => setAssignee(e.target.value)}
                placeholder="örn: ahmet"
              />
            </Field>
            <Field label="Priority">
              <Select
                value={priority}
                onChange={(e) =>
                  setPriority(e.target.value as WorkOrderPriority)
                }
              >
                {WORK_ORDER_PRIORITIES.map((p) => (
                  <option key={p} value={p}>
                    {PRIORITY_LABELS[p]}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="ETA (yerel)">
              <TextInput
                type="datetime-local"
                value={eta}
                onChange={(e) => setEta(e.target.value)}
              />
            </Field>
          </Toolbar>
          <Toolbar>
            <Button onClick={saveAssignee} disabled={busy}>
              Atamayı Kaydet
            </Button>
            <Button
              variant="secondary"
              onClick={updatePriorityOrETA}
              disabled={busy}
            >
              Priority / ETA Kaydet
            </Button>
            {wo.status !== "in_progress" ? (
              <Button
                variant="secondary"
                onClick={() => transition("in_progress")}
                disabled={busy}
              >
                İşleme Al
              </Button>
            ) : null}
            <Button onClick={resolve} disabled={busy}>
              Çözüldü Olarak İşaretle
            </Button>
            <Button variant="danger" onClick={cancelWO} disabled={busy}>
              İptal Et
            </Button>
          </Toolbar>
          <Field label="Not">
            <TextArea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="Aksiyona iliştirilecek not (opsiyonel)…"
            />
          </Field>
          <Button variant="secondary" onClick={addNote} disabled={busy}>
            Not Ekle (yalnız note_added event'i)
          </Button>
        </section>
      ) : (
        <div className="banner">
          Bu iş emri terminal durumda ({STATUS_LABELS[wo.status]}). Yeni
          aksiyon alınamaz.
        </div>
      )}

      <section className="section">
        <h3 className="section-title">Olay Geçmişi ({events.length})</h3>
        {events.length === 0 ? (
          <div className="empty">Henüz olay yok.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Zaman</th>
                <th>Tip</th>
                <th>Eski → Yeni</th>
                <th>Aktör</th>
                <th>Not</th>
              </tr>
            </thead>
            <tbody>
              {events.map((ev) => (
                <tr key={ev.id}>
                  <td>{new Date(ev.created_at).toLocaleString("tr-TR")}</td>
                  <td>
                    <code style={{ fontSize: 11 }}>{ev.event_type}</code>
                  </td>
                  <td>
                    {ev.old_value || "—"} → {ev.new_value || "—"}
                  </td>
                  <td>{ev.actor}</td>
                  <td>{ev.note ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
