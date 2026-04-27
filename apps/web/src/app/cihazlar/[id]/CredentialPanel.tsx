"use client";
import { useEffect, useState, FormEvent } from "react";
import {
  api,
  ApiError,
  CredentialProfile,
  DeviceCredentialBinding,
  TRANSPORTS,
  CRED_PURPOSES
} from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextInput } from "@/components/Field";

export function CredentialPanel({ deviceId }: { deviceId: string }) {
  const [list, setList] = useState<DeviceCredentialBinding[]>([]);
  const [profiles, setProfiles] = useState<CredentialProfile[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    profile_id: "",
    transport: "api-ssl",
    purpose: "primary",
    priority: 100
  });

  async function reload() {
    setError(null);
    try {
      const [b, p] = await Promise.all([
        api.get<{ data: DeviceCredentialBinding[] }>(
          `/api/v1/devices/${deviceId}/credentials`
        ),
        api.get<{ data: CredentialProfile[] }>(`/api/v1/credential-profiles`)
      ]);
      setList(b.data ?? []);
      setProfiles(p.data ?? []);
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 503
          ? "Veritabanı bağlı değil."
          : e instanceof Error
            ? e.message
            : "Bilinmeyen hata";
      setError(msg);
    }
  }

  useEffect(() => { reload(); }, [deviceId]);

  async function bind(e: FormEvent) {
    e.preventDefault();
    if (!form.profile_id) {
      setError("Bir kimlik profili seçin.");
      return;
    }
    try {
      await api.put(`/api/v1/devices/${deviceId}/credentials`, {
        credential_profile_id: form.profile_id,
        transport: form.transport,
        purpose: form.purpose,
        priority: Number(form.priority),
        enabled: true
      });
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Kayıt başarısız");
    }
  }

  async function remove(profileId: string) {
    if (!confirm("Profil bağlantısı kaldırılsın mı?")) return;
    try {
      await api.del(`/api/v1/devices/${deviceId}/credentials/${profileId}`);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Silinemedi");
    }
  }

  return (
    <section className="section">
      <h3 className="section-title">Kimlik Bilgileri</h3>
      <p style={{ color: "var(--text-dim)", fontSize: 12, marginTop: 0 }}>
        Probe/Poll çağrısı bu listedeki en yüksek öncelikli (en küçük priority)
        ve enabled bağlantıyı seçer. Sırlar UI'da gösterilmez.
      </p>
      {error ? <div className="banner">{error}</div> : null}

      <table>
        <thead>
          <tr>
            <th>Profil</th>
            <th>Tip</th>
            <th>Transport</th>
            <th>Purpose</th>
            <th>Priority</th>
            <th>Enabled</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {list.length === 0 ? (
            <tr><td colSpan={7} className="empty">Henüz kimlik bağlantısı yok.</td></tr>
          ) : null}
          {list.map((b) => (
            <tr key={b.credential_profile_id + b.transport}>
              <td>{b.profile_name ?? b.credential_profile_id}</td>
              <td><span className="badge">{b.auth_type ?? "—"}</span></td>
              <td>{b.transport}</td>
              <td>{b.purpose}</td>
              <td>{b.priority}</td>
              <td>
                <span className={`badge ${b.enabled ? "good" : "warn"}`}>
                  {b.enabled ? "yes" : "no"}
                </span>
              </td>
              <td>
                <Button variant="danger" onClick={() => remove(b.credential_profile_id)}>
                  Kaldır
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <form
        onSubmit={bind}
        style={{ display: "grid", gridTemplateColumns: "2fr 1fr 1fr 1fr auto", gap: 8, marginTop: 12, alignItems: "end" }}
      >
        <Field label="Profil">
          <Select
            value={form.profile_id}
            onChange={(e) => setForm({ ...form, profile_id: e.target.value })}
          >
            <option value="">— seç —</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} ({p.auth_type})
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Transport">
          <Select
            value={form.transport}
            onChange={(e) => setForm({ ...form, transport: e.target.value })}
          >
            {TRANSPORTS.map((t) => <option key={t} value={t}>{t}</option>)}
          </Select>
        </Field>
        <Field label="Purpose">
          <Select
            value={form.purpose}
            onChange={(e) => setForm({ ...form, purpose: e.target.value })}
          >
            {CRED_PURPOSES.map((t) => <option key={t} value={t}>{t}</option>)}
          </Select>
        </Field>
        <Field label="Priority">
          <TextInput
            type="number"
            value={String(form.priority)}
            onChange={(e) => setForm({ ...form, priority: Number(e.target.value) })}
          />
        </Field>
        <Toolbar>
          <Button type="submit">Bağla</Button>
        </Toolbar>
      </form>
    </section>
  );
}
