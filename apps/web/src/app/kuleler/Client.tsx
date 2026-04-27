"use client";
import { useEffect, useState, FormEvent } from "react";
import { api, Tower, Site, ApiError } from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextArea, TextInput } from "@/components/Field";
import { Modal } from "@/components/Modal";

export function TowersClient() {
  const [list, setList] = useState<Tower[] | null>(null);
  const [sites, setSites] = useState<Site[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({
    name: "",
    site_id: "",
    code: "",
    height_m: "",
    notes: ""
  });

  async function reload() {
    try {
      const [t, s] = await Promise.all([
        api.get<{ data: Tower[] }>("/api/v1/towers"),
        api.get<{ data: Site[] }>("/api/v1/sites").catch(() => ({ data: [] }))
      ]);
      setList(t.data ?? []);
      setSites(s.data ?? []);
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
      await api.post("/api/v1/towers", {
        name: form.name,
        site_id: form.site_id || undefined,
        code: form.code || undefined,
        height_m: form.height_m ? Number(form.height_m) : undefined,
        notes: form.notes || undefined
      });
      setOpen(false);
      setForm({ name: "", site_id: "", code: "", height_m: "", notes: "" });
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "İşlem başarısız");
    }
  }

  return (
    <div>
      <Toolbar>
        <Button onClick={() => setOpen(true)}>+ Yeni Kule</Button>
        <Button variant="secondary" onClick={reload}>
          Yenile
        </Button>
      </Toolbar>
      {error ? <div className="banner">{error}</div> : null}
      <table>
        <thead>
          <tr>
            <th>Kule</th>
            <th>Kod</th>
            <th>Bölge</th>
            <th>Yükseklik (m)</th>
            <th>Oluşturuldu</th>
          </tr>
        </thead>
        <tbody>
          {list && list.length === 0 ? (
            <tr>
              <td colSpan={5} className="empty">
                Kule kaydı yok.
              </td>
            </tr>
          ) : null}
          {(list ?? []).map((t) => {
            const site = sites.find((s) => s.id === t.site_id);
            return (
              <tr key={t.id}>
                <td>{t.name}</td>
                <td>{t.code ?? "—"}</td>
                <td>{site?.name ?? "—"}</td>
                <td>{t.height_m ?? "—"}</td>
                <td>{new Date(t.created_at).toLocaleDateString("tr-TR")}</td>
              </tr>
            );
          })}
        </tbody>
      </table>

      <Modal open={open} onClose={() => setOpen(false)} title="Yeni Kule">
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
          <Field label="Kod">
            <TextInput
              value={form.code}
              onChange={(e) => setForm({ ...form, code: e.target.value })}
            />
          </Field>
          <Field label="Bölge">
            <Select
              value={form.site_id}
              onChange={(e) => setForm({ ...form, site_id: e.target.value })}
            >
              <option value="">— seç —</option>
              {sites.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Yükseklik (m)">
            <TextInput
              type="number"
              step="0.1"
              value={form.height_m}
              onChange={(e) => setForm({ ...form, height_m: e.target.value })}
            />
          </Field>
          <div style={{ gridColumn: "1 / -1" }}>
            <Field label="Notlar">
              <TextArea
                value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })}
              />
            </Field>
          </div>
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
