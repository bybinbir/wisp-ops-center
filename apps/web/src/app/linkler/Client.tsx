"use client";
import { useEffect, useState, FormEvent } from "react";
import { api, Link, Device, ApiError } from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextInput } from "@/components/Field";
import { Modal } from "@/components/Modal";

export function LinksClient() {
  const [list, setList] = useState<Link[] | null>(null);
  const [devices, setDevices] = useState<Device[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({
    name: "",
    topology: "ptp",
    master_device_id: "",
    frequency_mhz: "",
    channel_width_mhz: ""
  });

  async function reload() {
    try {
      const [l, d] = await Promise.all([
        api.get<{ data: Link[] }>("/api/v1/links"),
        api
          .get<{ data: Device[] }>("/api/v1/devices")
          .catch(() => ({ data: [] }))
      ]);
      setList(l.data ?? []);
      setDevices(d.data ?? []);
      setError(null);
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 503
          ? "Veritabanı bağlı değil."
          : e instanceof Error
            ? e.message
            : "Bilinmeyen hata";
      setError(msg);
      setList([]);
    }
  }

  useEffect(() => {
    reload();
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      await api.post("/api/v1/links", {
        name: form.name,
        topology: form.topology,
        master_device_id: form.master_device_id,
        frequency_mhz: form.frequency_mhz
          ? Number(form.frequency_mhz)
          : undefined,
        channel_width_mhz: form.channel_width_mhz
          ? Number(form.channel_width_mhz)
          : undefined
      });
      setOpen(false);
      setForm({
        name: "",
        topology: "ptp",
        master_device_id: "",
        frequency_mhz: "",
        channel_width_mhz: ""
      });
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "İşlem başarısız");
    }
  }

  return (
    <div>
      <Toolbar>
        <Button onClick={() => setOpen(true)}>+ Yeni Link</Button>
        <Button variant="secondary" onClick={reload}>
          Yenile
        </Button>
      </Toolbar>
      {error ? <div className="banner">{error}</div> : null}
      <table>
        <thead>
          <tr>
            <th>Ad</th>
            <th>Topoloji</th>
            <th>Master</th>
            <th>Frekans (MHz)</th>
            <th>Kanal (MHz)</th>
            <th>Risk</th>
          </tr>
        </thead>
        <tbody>
          {list && list.length === 0 ? (
            <tr>
              <td colSpan={6} className="empty">
                Link kaydı yok.
              </td>
            </tr>
          ) : null}
          {(list ?? []).map((l) => {
            const master = devices.find((d) => d.id === l.master_device_id);
            return (
              <tr key={l.id}>
                <td>{l.name}</td>
                <td>{l.topology}</td>
                <td>{master?.name ?? l.master_device_id}</td>
                <td>{l.frequency_mhz ?? "—"}</td>
                <td>{l.channel_width_mhz ?? "—"}</td>
                <td>
                  <span
                    className={`badge ${
                      l.risk === "healthy"
                        ? "good"
                        : l.risk === "watch"
                          ? "warn"
                          : "bad"
                    }`}
                  >
                    {l.risk}
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>

      <Modal open={open} onClose={() => setOpen(false)} title="Yeni Link">
        <form
          onSubmit={submit}
          style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}
        >
          <Field label="Ad *">
            <TextInput
              required
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </Field>
          <Field label="Topoloji *">
            <Select
              value={form.topology}
              onChange={(e) => setForm({ ...form, topology: e.target.value })}
            >
              <option value="ptp">PTP</option>
              <option value="ptmp">PTMP</option>
            </Select>
          </Field>
          <Field label="Master Cihaz *">
            <Select
              value={form.master_device_id}
              required
              onChange={(e) =>
                setForm({ ...form, master_device_id: e.target.value })
              }
            >
              <option value="">— seç —</option>
              {devices.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name} ({d.vendor})
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Frekans (MHz)">
            <TextInput
              type="number"
              value={form.frequency_mhz}
              onChange={(e) =>
                setForm({ ...form, frequency_mhz: e.target.value })
              }
            />
          </Field>
          <Field label="Kanal Genişliği (MHz)">
            <TextInput
              type="number"
              value={form.channel_width_mhz}
              onChange={(e) =>
                setForm({ ...form, channel_width_mhz: e.target.value })
              }
            />
          </Field>
          <div
            style={{
              gridColumn: "1 / -1",
              display: "flex",
              justifyContent: "flex-end",
              gap: 8
            }}
          >
            <Button
              type="button"
              variant="secondary"
              onClick={() => setOpen(false)}
            >
              İptal
            </Button>
            <Button type="submit">Oluştur</Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
