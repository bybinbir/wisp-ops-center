"use client";
import Link from "next/link";
import { useEffect, useState, FormEvent } from "react";
import {
  api,
  Device,
  Site,
  Tower,
  VENDORS,
  ROLES,
  DEVICE_STATUSES,
  ApiError
} from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextInput, TextArea } from "@/components/Field";
import { Modal } from "@/components/Modal";

type DeviceForm = {
  name: string;
  vendor: string;
  role: string;
  ip_address: string;
  site_id: string;
  tower_id: string;
  model: string;
  os_version: string;
  firmware_version: string;
  status: string;
  tags: string;
  notes: string;
};

const empty: DeviceForm = {
  name: "",
  vendor: "mikrotik",
  role: "ap",
  ip_address: "",
  site_id: "",
  tower_id: "",
  model: "",
  os_version: "",
  firmware_version: "",
  status: "active",
  tags: "",
  notes: ""
};

export function DevicesClient() {
  const [list, setList] = useState<Device[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [sites, setSites] = useState<Site[]>([]);
  const [towers, setTowers] = useState<Tower[]>([]);
  const [editing, setEditing] = useState<Device | null>(null);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState<DeviceForm>(empty);
  const [busyId, setBusyId] = useState<string | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const [d, s, t] = await Promise.all([
        api.get<{ data: Device[] }>("/api/v1/devices"),
        api.get<{ data: Site[] }>("/api/v1/sites").catch(() => ({ data: [] })),
        api.get<{ data: Tower[] }>("/api/v1/towers").catch(() => ({ data: [] }))
      ]);
      setList(d.data ?? []);
      setSites(s.data ?? []);
      setTowers(t.data ?? []);
      setError(null);
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 503
          ? "Veritabanı bağlı değil. WISP_DATABASE_URL ayarlayıp migration çalıştırın."
          : e instanceof Error
            ? e.message
            : "Bilinmeyen hata";
      setError(msg);
      setList([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
  }, []);

  function openCreate() {
    setForm(empty);
    setEditing(null);
    setCreating(true);
  }

  function openEdit(d: Device) {
    setEditing(d);
    setCreating(false);
    setForm({
      name: d.name,
      vendor: d.vendor,
      role: d.role,
      ip_address: d.ip_address ?? "",
      site_id: d.site_id ?? "",
      tower_id: d.tower_id ?? "",
      model: d.model ?? "",
      os_version: d.os_version ?? "",
      firmware_version: d.firmware_version ?? "",
      status: d.status,
      tags: (d.tags ?? []).join(", "),
      notes: d.notes ?? ""
    });
  }

  function close() {
    setCreating(false);
    setEditing(null);
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const payload = {
      name: form.name,
      vendor: form.vendor,
      role: form.role,
      ip_address: form.ip_address || undefined,
      site_id: form.site_id || undefined,
      tower_id: form.tower_id || undefined,
      model: form.model || undefined,
      os_version: form.os_version || undefined,
      firmware_version: form.firmware_version || undefined,
      status: form.status,
      tags: form.tags
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean),
      notes: form.notes || undefined
    };
    try {
      if (editing) {
        await api.patch(`/api/v1/devices/${editing.id}`, payload);
      } else {
        await api.post("/api/v1/devices", payload);
      }
      close();
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "İşlem başarısız");
    }
  }

  async function remove(id: string) {
    if (!confirm("Bu cihazı silmek istediğinize emin misiniz?")) return;
    try {
      await api.del(`/api/v1/devices/${id}`);
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Silinemedi");
    }
  }

  async function probe(id: string) {
    setBusyId(id);
    try {
      const r = await api.post<{ error?: string }>(
        `/api/v1/devices/${id}/probe`
      );
      if (r.error) alert("Probe sonucu: " + r.error);
      else alert("Probe başarılı.");
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Probe başarısız");
    } finally {
      setBusyId(null);
    }
  }

  async function poll(id: string) {
    setBusyId(id);
    try {
      const r = await api.post<{ error?: string }>(
        `/api/v1/devices/${id}/poll`
      );
      if (r.error) alert("Poll sonucu: " + r.error);
      else alert("Read-only poll başarılı.");
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Poll başarısız");
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div>
      <div className="banner">
        <strong>Faz 4:</strong> MikroTik ve Mimosa cihazlar için Probe + Read-only
        Poll mevcut. Mimosa SNMPv2/v3 read-only; vendor MIB henüz doğrulanmadı,
        sonuç partial işaretlenir. AP→Client aktif testler Faz 5 sonrası.
        Frekans değişikliği ve config apply tüm cihazlarda kapalı.
      </div>

      <Toolbar>
        <Button onClick={openCreate}>+ Yeni Cihaz</Button>
        <Button variant="secondary" onClick={reload}>Yenile</Button>
        {loading ? <span style={{ color: "var(--text-dim)" }}>Yükleniyor…</span> : null}
      </Toolbar>

      {error ? (
        <div className="banner" style={{ borderColor: "rgba(239,83,80,0.4)" }}>
          {error}
        </div>
      ) : null}

      <table>
        <thead>
          <tr>
            <th>Ad</th>
            <th>Vendor</th>
            <th>Rol</th>
            <th>IP</th>
            <th>Model</th>
            <th>Durum</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {list && list.length === 0 ? (
            <tr>
              <td colSpan={7} className="empty">
                Cihaz kaydı yok. Yeni Cihaz butonuyla ekleyebilirsiniz.
              </td>
            </tr>
          ) : null}
          {(list ?? []).map((d) => (
            <tr key={d.id}>
              <td>
                <Link href={`/cihazlar/${d.id}`}>{d.name}</Link>
              </td>
              <td>
                <span className="badge">{d.vendor}</span>
              </td>
              <td>{d.role}</td>
              <td>{d.ip_address ?? "—"}</td>
              <td>{d.model ?? "—"}</td>
              <td>
                <span
                  className={`badge ${d.status === "active" ? "good" : "warn"}`}
                >
                  {d.status}
                </span>
              </td>
              <td style={{ whiteSpace: "nowrap" }}>
                {d.vendor === "mikrotik" ? (
                  <>
                    <Button
                      variant="secondary"
                      disabled={busyId === d.id}
                      onClick={() => probe(d.id)}
                    >
                      Probe
                    </Button>{" "}
                    <Button
                      disabled={busyId === d.id}
                      onClick={() => poll(d.id)}
                    >
                      Poll
                    </Button>{" "}
                  </>
                ) : null}
                <Button variant="secondary" onClick={() => openEdit(d)}>
                  Düzenle
                </Button>{" "}
                <Button variant="danger" onClick={() => remove(d.id)}>
                  Sil
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <Modal
        open={creating || editing !== null}
        onClose={close}
        title={editing ? `Cihazı Düzenle: ${editing.name}` : "Yeni Cihaz"}
      >
        <form
          onSubmit={submit}
          style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}
        >
          <Field label="Ad *">
            <TextInput required value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </Field>
          <Field label="IP">
            <TextInput value={form.ip_address} placeholder="192.168.1.1"
              onChange={(e) => setForm({ ...form, ip_address: e.target.value })} />
          </Field>
          <Field label="Vendor *">
            <Select value={form.vendor}
              onChange={(e) => setForm({ ...form, vendor: e.target.value })}>
              {VENDORS.map((v) => <option key={v} value={v}>{v}</option>)}
            </Select>
          </Field>
          <Field label="Rol *">
            <Select value={form.role}
              onChange={(e) => setForm({ ...form, role: e.target.value })}>
              {ROLES.map((r) => <option key={r} value={r}>{r}</option>)}
            </Select>
          </Field>
          <Field label="Bölge">
            <Select value={form.site_id}
              onChange={(e) => setForm({ ...form, site_id: e.target.value })}>
              <option value="">— seç —</option>
              {sites.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </Select>
          </Field>
          <Field label="Kule">
            <Select value={form.tower_id}
              onChange={(e) => setForm({ ...form, tower_id: e.target.value })}>
              <option value="">— seç —</option>
              {towers.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
            </Select>
          </Field>
          <Field label="Model">
            <TextInput value={form.model}
              onChange={(e) => setForm({ ...form, model: e.target.value })} />
          </Field>
          <Field label="Firmware">
            <TextInput value={form.firmware_version}
              onChange={(e) => setForm({ ...form, firmware_version: e.target.value })} />
          </Field>
          <Field label="OS / RouterOS">
            <TextInput value={form.os_version}
              onChange={(e) => setForm({ ...form, os_version: e.target.value })} />
          </Field>
          <Field label="Durum">
            <Select value={form.status}
              onChange={(e) => setForm({ ...form, status: e.target.value })}>
              {DEVICE_STATUSES.map((s) => <option key={s} value={s}>{s}</option>)}
            </Select>
          </Field>
          <Field label="Etiketler (virgülle)">
            <TextInput value={form.tags} placeholder="koy1, kritik"
              onChange={(e) => setForm({ ...form, tags: e.target.value })} />
          </Field>
          <div style={{ gridColumn: "1 / -1" }}>
            <Field label="Notlar">
              <TextArea value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })} />
            </Field>
          </div>
          <div style={{ gridColumn: "1 / -1", display: "flex", justifyContent: "flex-end", gap: 8, marginTop: 8 }}>
            <Button type="button" variant="secondary" onClick={close}>İptal</Button>
            <Button type="submit">{editing ? "Kaydet" : "Oluştur"}</Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
