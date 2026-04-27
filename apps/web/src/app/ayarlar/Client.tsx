"use client";
import { useEffect, useState, FormEvent } from "react";
import {
  api,
  AUTH_TYPES,
  CredentialProfile,
  ApiError,
  SNMPV3_SECURITY_LEVELS,
  SNMPV3_AUTH_PROTOCOLS,
  SNMPV3_PRIV_PROTOCOLS,
  SSH_HOST_KEY_POLICIES
} from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextArea, TextInput } from "@/components/Field";
import { Modal } from "@/components/Modal";

type Form = {
  name: string;
  auth_type: string;
  username: string;
  secret: string;
  port: string;
  notes: string;
  snmpv3_username: string;
  snmpv3_security_level: string;
  snmpv3_auth_protocol: string;
  snmpv3_auth_secret: string;
  snmpv3_priv_protocol: string;
  snmpv3_priv_secret: string;
  verify_tls: boolean;
  ssh_host_key_policy: string;
  ssh_host_key_fingerprint: string;
};

const empty: Form = {
  name: "",
  auth_type: "routeros_api_ssl",
  username: "",
  secret: "",
  port: "",
  notes: "",
  snmpv3_username: "",
  snmpv3_security_level: "noAuthNoPriv",
  snmpv3_auth_protocol: "SHA",
  snmpv3_auth_secret: "",
  snmpv3_priv_protocol: "AES",
  snmpv3_priv_secret: "",
  verify_tls: false,
  ssh_host_key_policy: "insecure_ignore",
  ssh_host_key_fingerprint: ""
};

export function CredentialsClient() {
  const [list, setList] = useState<CredentialProfile[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<CredentialProfile | null>(null);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState<Form>(empty);
  const [secretMode, setSecretMode] = useState<"keep" | "set" | "clear">(
    "keep"
  );

  async function reload() {
    try {
      const r = await api.get<{ data: CredentialProfile[] }>(
        "/api/v1/credential-profiles"
      );
      setList(r.data ?? []);
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

  function openCreate() {
    setForm(empty);
    setEditing(null);
    setSecretMode("set");
    setCreating(true);
  }

  function openEdit(p: CredentialProfile) {
    setEditing(p);
    setCreating(false);
    setForm({
      name: p.name,
      auth_type: p.auth_type,
      username: p.username ?? "",
      secret: "",
      port: p.port ? String(p.port) : "",
      notes: p.notes ?? "",
      snmpv3_username: p.snmpv3_username ?? "",
      snmpv3_security_level: p.snmpv3_security_level ?? "noAuthNoPriv",
      snmpv3_auth_protocol: p.snmpv3_auth_protocol ?? "SHA",
      snmpv3_auth_secret: "",
      snmpv3_priv_protocol: p.snmpv3_priv_protocol ?? "AES",
      snmpv3_priv_secret: "",
      verify_tls: !!p.verify_tls,
      ssh_host_key_policy: p.ssh_host_key_policy ?? "insecure_ignore",
      ssh_host_key_fingerprint: p.ssh_host_key_fingerprint ?? ""
    });
    setSecretMode("keep");
  }

  function close() {
    setCreating(false);
    setEditing(null);
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const base: Record<string, unknown> = {
      name: form.name,
      auth_type: form.auth_type,
      username: form.username || undefined,
      port: form.port ? Number(form.port) : undefined,
      notes: form.notes || undefined,
      snmpv3_username: form.snmpv3_username || undefined,
      snmpv3_security_level: form.snmpv3_security_level || undefined,
      snmpv3_auth_protocol: form.snmpv3_auth_protocol || undefined,
      snmpv3_auth_secret: form.snmpv3_auth_secret || undefined,
      snmpv3_priv_protocol: form.snmpv3_priv_protocol || undefined,
      snmpv3_priv_secret: form.snmpv3_priv_secret || undefined,
      verify_tls: form.verify_tls,
      ssh_host_key_policy: form.ssh_host_key_policy,
      ssh_host_key_fingerprint: form.ssh_host_key_fingerprint || undefined
    };
    if (creating) {
      base.secret = form.secret || undefined;
    } else {
      if (secretMode === "set") base.secret = form.secret;
      else if (secretMode === "clear") base.secret = "";
      // "keep" mode: omit secret entirely
    }
    try {
      if (editing) {
        await api.patch(`/api/v1/credential-profiles/${editing.id}`, base);
      } else {
        await api.post("/api/v1/credential-profiles", base);
      }
      close();
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "İşlem başarısız");
    }
  }

  async function remove(id: string) {
    if (!confirm("Profil silinsin mi?")) return;
    try {
      await api.del(`/api/v1/credential-profiles/${id}`);
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Silinemedi");
    }
  }

  return (
    <div>
      <section className="section">
        <h3 className="section-title">Kimlik Profilleri</h3>
        <div className="banner">
          Sırlar AES-GCM ile <strong>WISP_VAULT_KEY</strong> kullanılarak
          şifrelenir. API cevapları ham parolayı içermez; yalnızca{" "}
          <span className="kbd">secret_set</span> bayrağı gösterilir. Audit
          kayıtları da ham parolayı içermez.
        </div>
        <Toolbar>
          <Button onClick={openCreate}>+ Yeni Profil</Button>
          <Button variant="secondary" onClick={reload}>
            Yenile
          </Button>
        </Toolbar>
        {error ? <div className="banner">{error}</div> : null}
        <table>
          <thead>
            <tr>
              <th>Ad</th>
              <th>Tip</th>
              <th>Kullanıcı</th>
              <th>Port</th>
              <th>Sır</th>
              <th>Güncelleme</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list && list.length === 0 ? (
              <tr>
                <td colSpan={7} className="empty">
                  Profil yok.
                </td>
              </tr>
            ) : null}
            {(list ?? []).map((p) => (
              <tr key={p.id}>
                <td>{p.name}</td>
                <td>
                  <span className="badge">{p.auth_type}</span>
                </td>
                <td>{p.username ?? "—"}</td>
                <td>{p.port ?? "—"}</td>
                <td>
                  <span className={`badge ${p.secret_set ? "good" : "warn"}`}>
                    {p.secret_set ? "tanımlı" : "boş"}
                  </span>
                </td>
                <td>{new Date(p.updated_at).toLocaleString("tr-TR")}</td>
                <td style={{ whiteSpace: "nowrap" }}>
                  <Button variant="secondary" onClick={() => openEdit(p)}>
                    Düzenle
                  </Button>{" "}
                  <Button variant="danger" onClick={() => remove(p.id)}>
                    Sil
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <Modal
        open={creating || editing !== null}
        onClose={close}
        title={editing ? `Profil Düzenle: ${editing.name}` : "Yeni Profil"}
      >
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
          <Field label="Tip *">
            <Select
              value={form.auth_type}
              onChange={(e) =>
                setForm({ ...form, auth_type: e.target.value })
              }
            >
              {AUTH_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Kullanıcı">
            <TextInput
              value={form.username}
              onChange={(e) => setForm({ ...form, username: e.target.value })}
            />
          </Field>
          <Field label="Port">
            <TextInput
              type="number"
              value={form.port}
              onChange={(e) => setForm({ ...form, port: e.target.value })}
            />
          </Field>
          <div style={{ gridColumn: "1 / -1", color: "var(--text-dim)", fontSize: 11, marginTop: 8 }}>
            SNMPv3 + Transport sertleştirme (opsiyonel)
          </div>
          <Field label="SNMPv3 Kullanıcı">
            <TextInput
              value={form.snmpv3_username}
              onChange={(e) => setForm({ ...form, snmpv3_username: e.target.value })}
            />
          </Field>
          <Field label="SNMPv3 Security Level">
            <Select
              value={form.snmpv3_security_level}
              onChange={(e) => setForm({ ...form, snmpv3_security_level: e.target.value })}
            >
              {SNMPV3_SECURITY_LEVELS.map((s) => <option key={s} value={s}>{s}</option>)}
            </Select>
          </Field>
          <Field label="SNMPv3 Auth Protocol">
            <Select
              value={form.snmpv3_auth_protocol}
              onChange={(e) => setForm({ ...form, snmpv3_auth_protocol: e.target.value })}
            >
              {SNMPV3_AUTH_PROTOCOLS.map((s) => <option key={s} value={s}>{s}</option>)}
            </Select>
          </Field>
          <Field label="SNMPv3 Auth Secret">
            <TextInput
              type="password"
              value={form.snmpv3_auth_secret}
              onChange={(e) => setForm({ ...form, snmpv3_auth_secret: e.target.value })}
            />
          </Field>
          <Field label="SNMPv3 Priv Protocol">
            <Select
              value={form.snmpv3_priv_protocol}
              onChange={(e) => setForm({ ...form, snmpv3_priv_protocol: e.target.value })}
            >
              {SNMPV3_PRIV_PROTOCOLS.map((s) => <option key={s} value={s}>{s}</option>)}
            </Select>
          </Field>
          <Field label="SNMPv3 Priv Secret">
            <TextInput
              type="password"
              value={form.snmpv3_priv_secret}
              onChange={(e) => setForm({ ...form, snmpv3_priv_secret: e.target.value })}
            />
          </Field>
          <Field label="TLS Doğrulama (RouterOS)">
            <Select
              value={form.verify_tls ? "true" : "false"}
              onChange={(e) => setForm({ ...form, verify_tls: e.target.value === "true" })}
            >
              <option value="false">Devre dışı (varsayılan)</option>
              <option value="true">Aktif (CA gerekli)</option>
            </Select>
          </Field>
          <Field label="SSH Host Key Policy">
            <Select
              value={form.ssh_host_key_policy}
              onChange={(e) => setForm({ ...form, ssh_host_key_policy: e.target.value })}
            >
              {SSH_HOST_KEY_POLICIES.map((s) => <option key={s} value={s}>{s}</option>)}
            </Select>
          </Field>
          <Field label="Host Key Fingerprint (opsiyonel)">
            <TextInput
              value={form.ssh_host_key_fingerprint}
              onChange={(e) => setForm({ ...form, ssh_host_key_fingerprint: e.target.value })}
            />
          </Field>
          {creating ? (
            <div style={{ gridColumn: "1 / -1" }}>
              <Field
                label="Sır"
                help="Kaydedildikten sonra bir daha gösterilmez. AES-GCM ile şifrelenir."
              >
                <TextInput
                  type="password"
                  value={form.secret}
                  onChange={(e) =>
                    setForm({ ...form, secret: e.target.value })
                  }
                />
              </Field>
            </div>
          ) : (
            <div style={{ gridColumn: "1 / -1" }}>
              <Field label="Sır Modu">
                <Select
                  value={secretMode}
                  onChange={(e) =>
                    setSecretMode(e.target.value as typeof secretMode)
                  }
                >
                  <option value="keep">Mevcut sırrı koru</option>
                  <option value="set">Yeni sır ata</option>
                  <option value="clear">Sırrı temizle</option>
                </Select>
              </Field>
              {secretMode === "set" ? (
                <div style={{ marginTop: 8 }}>
                  <Field label="Yeni Sır">
                    <TextInput
                      type="password"
                      value={form.secret}
                      onChange={(e) =>
                        setForm({ ...form, secret: e.target.value })
                      }
                    />
                  </Field>
                </div>
              ) : null}
            </div>
          )}
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
            <Button type="button" variant="secondary" onClick={close}>
              İptal
            </Button>
            <Button type="submit">{editing ? "Kaydet" : "Oluştur"}</Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
