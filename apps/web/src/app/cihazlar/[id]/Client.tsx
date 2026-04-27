"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import {
  api,
  Device,
  PollResult,
  PollSnapshot,
  ProbeResult,
  ApiError,
  APHealthRow
} from "@/lib/api";
import { Button, Toolbar } from "@/components/Toolbar";
import { CredentialPanel } from "./CredentialPanel";

type WirelessClient = {
  interface: string;
  mac: string;
  ip?: string;
  ssid?: string;
  signal_dbm?: number | null;
  snr_db?: number | null;
  tx_rate_mbps?: number | null;
  rx_rate_mbps?: number | null;
  ccq?: number | null;
};

type Interface = {
  name: string;
  type?: string;
  running: boolean;
  disabled: boolean;
  rx_bytes?: number | null;
  tx_bytes?: number | null;
};

export function DeviceDetailClient({ deviceId }: { deviceId: string }) {
  const [device, setDevice] = useState<Device | null>(null);
  const [latest, setLatest] = useState<PollResult | null>(null);
  const [history, setHistory] = useState<PollResult[]>([]);
  const [clients, setClients] = useState<WirelessClient[]>([]);
  const [ifaces, setIfaces] = useState<Interface[]>([]);
  const [busy, setBusy] = useState<"" | "probe" | "poll">("");
  const [lastProbe, setLastProbe] = useState<ProbeResult | null>(null);
  const [lastPoll, setLastPoll] = useState<PollSnapshot | null>(null);
  const [apHealth, setApHealth] = useState<APHealthRow | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    setError(null);
    try {
      const [d, t, c, i, h] = await Promise.all([
        api.get<{ data: Device }>(`/api/v1/devices/${deviceId}`),
        api.get<{ data: PollResult | null }>(
          `/api/v1/devices/${deviceId}/telemetry/latest`
        ),
        api.get<{ data: WirelessClient[] }>(
          `/api/v1/devices/${deviceId}/wireless-clients/latest`
        ),
        api.get<{ data: Interface[] }>(
          `/api/v1/devices/${deviceId}/interfaces/latest`
        ),
        api.get<{ data: PollResult[] }>(`/api/v1/mikrotik/poll-results`)
      ]);
      setDevice(d.data);
      setLatest(t.data ?? null);
      setClients(c.data ?? []);
      setIfaces(i.data ?? []);
      setHistory((h.data ?? []).filter((r) => r.device_id === deviceId).slice(0, 20));
      // AP-health is best-effort: tüm cihazlar AP değil; 404 sessizce yok sayılır.
      try {
        const ap = await api.get<{ data: APHealthRow }>(
          `/api/v1/devices/${deviceId}/ap-health`
        );
        setApHealth(ap.data);
      } catch {
        setApHealth(null);
      }
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 503
          ? "Veritabanı bağlı değil. WISP_DATABASE_URL ayarlayın."
          : e instanceof Error
            ? e.message
            : "Bilinmeyen hata";
      setError(msg);
    }
  }

  useEffect(() => {
    load();
  }, [deviceId]);

  async function runProbe() {
    setBusy("probe");
    setError(null);
    try {
      const r = await api.post<{ data: ProbeResult; error?: string }>(
        `/api/v1/devices/${deviceId}/probe`
      );
      setLastProbe(r.data);
      if (r.error) setError(r.error);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Probe başarısız");
    } finally {
      setBusy("");
    }
  }

  async function runPoll() {
    setBusy("poll");
    setError(null);
    try {
      const r = await api.post<{ data: PollSnapshot; error?: string }>(
        `/api/v1/devices/${deviceId}/poll`
      );
      setLastPoll(r.data);
      if (r.error) setError(r.error);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Poll başarısız");
    } finally {
      setBusy("");
    }
  }

  if (!device) {
    return (
      <div>
        {error ? <div className="banner">{error}</div> : null}
        <p className="empty">Yükleniyor…</p>
        <Link href="/cihazlar" className="sidebar-link" style={{ display: "inline-block", marginTop: 8 }}>
          ← Cihazlar listesine dön
        </Link>
      </div>
    );
  }

  const canProbe = device.vendor === "mikrotik" || device.vendor === "mimosa";

  return (
    <div>
      <div className="banner">
        <strong>Faz 4 salt-okuma uyarısı:</strong> Probe ve Poll yalnızca
        veri okur. Mimosa Faz 4 yalnızca standart SNMP veriyi toplar;
        vendor MIB doğrulanmadığı için sonuç partial işaretlenir.
        Frekans değiştirme, bandwidth-test ve config apply kapalıdır.
      </div>

      <Toolbar>
        <Link href="/cihazlar" className="sidebar-link">← Liste</Link>
        {canProbe ? (
          <>
            <Button
              disabled={busy !== ""}
              onClick={runProbe}
            >
              {busy === "probe" ? "Probing…" : "Probe"}
            </Button>
            <Button
              variant="secondary"
              disabled={busy !== ""}
              onClick={runPoll}
            >
              {busy === "poll" ? "Polling…" : "Read-only Poll"}
            </Button>
          </>
        ) : (
          <span style={{ color: "var(--text-dim)", fontSize: 13 }}>
            Probe/Poll Faz 4'te MikroTik ve Mimosa cihazlarda kullanılabilir.
          </span>
        )}
        <Button variant="secondary" onClick={load}>Yenile</Button>
      </Toolbar>

      {error ? <div className="banner">{error}</div> : null}

      {apHealth ? (
        <section className="section">
          <h3 className="section-title">AP Sağlık Skoru (Faz 6)</h3>
          <div className="cards">
            <div className="card">
              <p className="card-title">AP Skoru</p>
              <p
                className="card-value"
                style={{
                  color:
                    apHealth.severity === "critical"
                      ? "#ff6b6b"
                      : apHealth.severity === "warning"
                        ? "#f4b400"
                        : "#4caf50",
                }}
              >
                {apHealth.ap_score}
              </p>
              <div className="card-meta">{apHealth.severity.toUpperCase()}</div>
            </div>
            <div className="card">
              <p className="card-title">Etkilenen Müşteri</p>
              <p className="card-value">
                {apHealth.critical_customers + apHealth.warning_customers}
              </p>
              <div className="card-meta">
                {apHealth.critical_customers} kritik /{" "}
                {apHealth.warning_customers} uyarı /{" "}
                {apHealth.healthy_customers} sağlıklı / toplam{" "}
                {apHealth.total_customers}
              </div>
            </div>
            <div className="card">
              <p className="card-title">AP-Wide Tanı</p>
              <p className="card-value" style={{ fontSize: 14 }}>
                {apHealth.is_ap_wide_interference
                  ? "AP geneli kötüleşme"
                  : "Tek müşteri sorunu olabilir"}
              </p>
              <div className="card-meta">
                degradation_ratio = {apHealth.degradation_ratio.toFixed(2)}
              </div>
              <div className="card-meta">
                {new Date(apHealth.calculated_at).toLocaleString("tr-TR")}
              </div>
            </div>
          </div>
          {apHealth.reasons && apHealth.reasons.length > 0 ? (
            <ul style={{ marginTop: 8, fontSize: 13 }}>
              {apHealth.reasons.map((r, i) => (
                <li key={i}>{r}</li>
              ))}
            </ul>
          ) : null}
        </section>
      ) : null}

      <section className="cards">
        <div className="card">
          <p className="card-title">Genel</p>
          <p className="card-value" style={{ fontSize: 18 }}>{device.name}</p>
          <div className="card-meta">
            <span className="badge">{device.vendor}</span>{" "}
            <span className="badge">{device.role}</span>{" "}
            <span className={`badge ${device.status === "active" ? "good" : "warn"}`}>{device.status}</span>
          </div>
          <div className="card-meta">IP: {device.ip_address ?? "—"}</div>
          <div className="card-meta">Model: {device.model ?? "—"}</div>
          <div className="card-meta">Firmware: {device.firmware_version ?? device.os_version ?? "—"}</div>
        </div>

        <div className="card">
          <p className="card-title">Son Poll</p>
          {latest ? (
            <>
              <p className="card-value" style={{ fontSize: 18 }}>{latest.status}</p>
              <div className="card-meta">{new Date(latest.finished_at).toLocaleString("tr-TR")}</div>
              <div className="card-meta">Süre: {latest.duration_ms} ms</div>
              <div className="card-meta">Transport: {latest.transport}</div>
              {latest.error_message ? (
                <div className="card-meta" style={{ color: "var(--bad)" }}>
                  {latest.error_message}
                </div>
              ) : null}
            </>
          ) : (
            <p className="card-meta">Henüz poll yok.</p>
          )}
        </div>

        <div className="card">
          <p className="card-title">Son Probe</p>
          {lastProbe ? (
            <>
              <p className="card-value" style={{ fontSize: 18 }}>
                {lastProbe.reachable ? "Erişildi" : "Erişilemedi"}
              </p>
              <div className="card-meta">{lastProbe.identity_name ?? "—"}</div>
              <div className="card-meta">{lastProbe.routeros_version ?? "—"}</div>
              <div className="card-meta">
                Wireless: {lastProbe.wireless_available ? lastProbe.wifi_package ?? "var" : "yok"}
              </div>
            </>
          ) : (
            <p className="card-meta">Bu oturumda probe çalıştırılmadı.</p>
          )}
        </div>
      </section>

      {lastPoll?.system ? (
        <section className="section">
          <h3 className="section-title">Son Sağlık (poll snapshot)</h3>
          <div className="cards">
            <div className="card">
              <p className="card-title">Uptime</p>
              <p className="card-value" style={{ fontSize: 18 }}>
                {typeof lastPoll.system?.uptime_sec === 'number' ? `${Math.round((lastPoll.system.uptime_sec as number) / 3600)} sa` : "—"}
              </p>
            </div>
            <div className="card">
              <p className="card-title">CPU</p>
              <p className="card-value" style={{ fontSize: 18 }}>
                {lastPoll.system?.cpu_load_pct != null ? `${String(lastPoll.system.cpu_load_pct)}%` : "—"}
              </p>
            </div>
            <div className="card">
              <p className="card-title">Sıcaklık</p>
              <p className="card-value" style={{ fontSize: 18 }}>
                {lastPoll.system?.temp_c != null ? `${String(lastPoll.system.temp_c)}°C` : "—"}
              </p>
            </div>
          </div>
        </section>
      ) : null}

      <section className="section">
        <h3 className="section-title">Arayüzler</h3>
        {ifaces.length === 0 ? (
          <p className="empty">Veri yok. Önce Read-only Poll çalıştırın.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Ad</th>
                <th>Tip</th>
                <th>Durum</th>
                <th>RX bytes</th>
                <th>TX bytes</th>
              </tr>
            </thead>
            <tbody>
              {ifaces.map((i) => (
                <tr key={i.name}>
                  <td>{i.name}</td>
                  <td>{i.type ?? "—"}</td>
                  <td>
                    <span className={`badge ${i.running ? "good" : "warn"}`}>
                      {i.disabled ? "disabled" : i.running ? "running" : "down"}
                    </span>
                  </td>
                  <td>{i.rx_bytes ?? "—"}</td>
                  <td>{i.tx_bytes ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="section">
        <h3 className="section-title">Kablosuz İstemciler</h3>
        {clients.length === 0 ? (
          <p className="empty">Kayıtlı kablosuz istemci yok.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Arayüz</th>
                <th>MAC</th>
                <th>IP</th>
                <th>SSID</th>
                <th>Sinyal</th>
                <th>SNR</th>
                <th>TX</th>
                <th>RX</th>
                <th>CCQ</th>
              </tr>
            </thead>
            <tbody>
              {clients.map((c, idx) => (
                <tr key={`${c.mac}-${idx}`}>
                  <td>{c.interface}</td>
                  <td>{c.mac}</td>
                  <td>{c.ip ?? "—"}</td>
                  <td>{c.ssid ?? "—"}</td>
                  <td>{c.signal_dbm != null ? `${c.signal_dbm} dBm` : "—"}</td>
                  <td>{c.snr_db != null ? `${c.snr_db}` : "—"}</td>
                  <td>{c.tx_rate_mbps != null ? `${c.tx_rate_mbps}` : "—"}</td>
                  <td>{c.rx_rate_mbps != null ? `${c.rx_rate_mbps}` : "—"}</td>
                  <td>{c.ccq != null ? `${c.ccq}` : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="section">
        <h3 className="section-title">Poll Geçmişi</h3>
        {history.length === 0 ? (
          <p className="empty">Geçmiş kayıt yok.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Zaman</th>
                <th>Op</th>
                <th>Transport</th>
                <th>Durum</th>
                <th>Süre</th>
                <th>Hata</th>
              </tr>
            </thead>
            <tbody>
              {history.map((h) => (
                <tr key={h.id}>
                  <td>{new Date(h.started_at).toLocaleString("tr-TR")}</td>
                  <td>{h.operation}</td>
                  <td>{h.transport}</td>
                  <td>
                    <span
                      className={`badge ${
                        h.status === "success"
                          ? "good"
                          : h.status === "partial"
                            ? "warn"
                            : "bad"
                      }`}
                    >
                      {h.status}
                    </span>
                  </td>
                  <td>{h.duration_ms} ms</td>
                  <td>{h.error_message ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      <CredentialPanel deviceId={deviceId} />
    </div>
  );
}
