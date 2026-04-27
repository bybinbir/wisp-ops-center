// Tek noktadan API çağrısı yardımcısı.

const BASE = process.env.NEXT_PUBLIC_API_BASE ?? "";
const TOKEN = process.env.NEXT_PUBLIC_API_TOKEN ?? "";

export type ApiResponse<T> = { data: T };

export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const url = path.startsWith("http") ? path : `${BASE}${path}`;
  const res = await fetch(url, {
    method,
    headers: {
      Accept: "application/json",
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {})
    },
    cache: "no-store",
    body: body !== undefined ? JSON.stringify(body) : undefined
  });
  if (res.status === 204) return undefined as T;
  let payload: unknown = null;
  try { payload = await res.json(); } catch { /* ignore */ }
  if (!res.ok) {
    const detail =
      typeof payload === "object" && payload !== null && "error" in payload
        ? String((payload as { error: unknown }).error)
        : `${res.status} ${res.statusText}`;
    throw new ApiError(detail, res.status, payload);
  }
  return payload as T;
}

export const api = {
  get: <T>(p: string) => request<T>("GET", p),
  post: <T>(p: string, body?: unknown) => request<T>("POST", p, body ?? {}),
  put: <T>(p: string, body: unknown) => request<T>("PUT", p, body),
  patch: <T>(p: string, body: unknown) => request<T>("PATCH", p, body),
  del: <T = void>(p: string) => request<T>("DELETE", p)
};

export type Device = {
  id: string; name: string; vendor: string; role: string;
  ip_address?: string; site_id?: string; tower_id?: string;
  model?: string; os_version?: string; firmware_version?: string;
  status: string; tags: string[]; notes?: string;
  created_at: string; updated_at: string;
};

export type Site = { id: string; name: string; code?: string; region?: string };
export type Tower = {
  id: string; name: string; site_id?: string; code?: string; height_m?: number;
  notes?: string; created_at: string; updated_at: string;
};

export type Link = {
  id: string; name: string; topology: string; master_device_id: string;
  frequency_mhz?: number; channel_width_mhz?: number; risk: string;
  last_checked_at?: string; created_at: string; updated_at: string;
};

export type CredentialProfile = {
  id: string; name: string; auth_type: string; username?: string;
  port?: number; secret_set: boolean; notes?: string;
  created_at: string; updated_at: string;
  snmpv3_username?: string;
  snmpv3_security_level?: string;
  snmpv3_auth_protocol?: string;
  snmpv3_auth_set?: boolean;
  snmpv3_priv_protocol?: string;
  snmpv3_priv_set?: boolean;
  verify_tls?: boolean;
  server_name_override?: string;
  ssh_host_key_policy?: string;
  ssh_host_key_fingerprint?: string;
};

export type DeviceCredentialBinding = {
  device_id: string;
  credential_profile_id: string;
  profile_name?: string;
  auth_type?: string;
  transport: string;
  purpose: string;
  priority: number;
  enabled: boolean;
  created_at: string;
};

export type PollResult = {
  id: number; device_id: string; vendor: string; operation: string;
  transport: string; status: string;
  started_at: string; finished_at: string; duration_ms: number;
  error_code?: string; error_message?: string;
  summary: Record<string, unknown>;
};

export type ProbeResult = {
  device_id: string; reachable: boolean; transport: string;
  identity_name?: string; routeros_version?: string; board?: string;
  architecture?: string; uptime_sec?: number;
  wireless_available: boolean; wifi_package?: string;
  // Mimosa-specific
  system_name?: string; system_descr?: string;
  model?: string; firmware?: string;
  vendor_mib_status?: string; partial?: boolean;
  error?: string; collected_at: string;
};

export type PollSnapshot = {
  device_id: string; transport: string;
  started_at: string; finished_at: string; duration_ms: number;
  system?: Record<string, unknown>;
  interfaces?: Array<Record<string, unknown>>;
  wireless_interfaces?: Array<Record<string, unknown>>;
  wireless_clients?: Array<Record<string, unknown>>;
  // Mimosa-specific
  radios?: Array<Record<string, unknown>>;
  links?: Array<Record<string, unknown>>;
  clients?: Array<Record<string, unknown>>;
  vendor_mib_status?: string;
  partial?: boolean;
  errors?: string[];
};

export type MimosaLatest = {
  clients: Array<Record<string, unknown>>;
  links: Array<Record<string, unknown>>;
};

export const VENDORS = ["mikrotik", "mimosa", "other", "unknown"] as const;
export const ROLES = ["ap", "cpe", "ptp_master", "ptp_slave", "router", "switch"] as const;
export const DEVICE_STATUSES = ["active", "retired", "maintenance", "spare"] as const;
export const AUTH_TYPES = [
  "routeros_api_ssl", "ssh", "snmp_v2", "snmp_v3", "mimosa_snmp", "vendor_api"
] as const;
export const SNMPV3_SECURITY_LEVELS = [
  "noAuthNoPriv", "authNoPriv", "authPriv"
] as const;
export const SNMPV3_AUTH_PROTOCOLS = ["MD5", "SHA", "SHA256"] as const;
export const SNMPV3_PRIV_PROTOCOLS = ["DES", "AES", "AES192", "AES256"] as const;
export const SSH_HOST_KEY_POLICIES = [
  "insecure_ignore", "trust_on_first_use", "pinned"
] as const;
export const TRANSPORTS = ["api-ssl", "ssh", "snmp", "vendor-api"] as const;
export const CRED_PURPOSES = ["primary", "api", "ssh", "snmp", "fallback"] as const;

// ─── Faz 6 — Customer Signal Scoring ─────────────────────────────────────

export const DIAGNOSES = [
  "healthy",
  "weak_customer_signal",
  "possible_cpe_alignment_issue",
  "ap_wide_interference",
  "ptp_link_degradation",
  "frequency_channel_risk",
  "high_latency",
  "packet_loss",
  "unstable_jitter",
  "device_offline",
  "stale_data",
  "data_insufficient",
] as const;
export type Diagnosis = (typeof DIAGNOSES)[number];

export const SEVERITIES = ["healthy", "warning", "critical", "unknown"] as const;
export type Severity = (typeof SEVERITIES)[number];

export const DIAGNOSIS_LABELS: Record<Diagnosis, string> = {
  healthy: "Sağlıklı",
  weak_customer_signal: "Zayıf Müşteri Sinyali",
  possible_cpe_alignment_issue: "CPE Yönlendirme Sorunu",
  ap_wide_interference: "AP Genelinde Parazit",
  ptp_link_degradation: "PtP Link Kötüleşmesi",
  frequency_channel_risk: "Frekans/Kanal Riski",
  high_latency: "Yüksek Gecikme",
  packet_loss: "Paket Kaybı",
  unstable_jitter: "Kararsız Jitter",
  device_offline: "Cihaz Çevrimdışı",
  stale_data: "Veri Bayat",
  data_insufficient: "Yetersiz Veri",
};

export const ACTION_LABELS: Record<string, string> = {
  no_action: "Aksiyon gerekmiyor",
  monitor: "Gözlem",
  schedule_field_visit: "Saha ziyareti planla",
  check_cpe_alignment: "CPE anten yönü kontrol",
  check_customer_cable: "Müşteri kablo/Ethernet kontrol",
  check_ap_interference: "AP parazit ölçümü",
  check_ptp_backhaul: "PtP backhaul kontrolü",
  review_frequency_plan: "Frekans planı gözden geçir",
  verify_power_or_ethernet: "Güç / Ethernet doğrulama",
  escalate_network_ops: "Network Ops'a yükselt",
};

export type CustomerWithIssue = {
  customer_id: string;
  customer_name: string;
  ap_device_id?: string;
  tower_id?: string;
  score: number;
  severity: Severity;
  diagnosis: Diagnosis;
  recommended_action: string;
  is_stale: boolean;
  calculated_at: string;
};

export type CustomerSignalScore = {
  id: string;
  customer_id: string;
  ap_device_id?: string;
  tower_id?: string;
  score: number;
  severity: Severity;
  diagnosis: Diagnosis;
  recommended_action: string;
  reasons: string[];
  contributing_metrics?: Record<string, number>;
  rssi_dbm?: number | null;
  snr_db?: number | null;
  ccq?: number | null;
  packet_loss_pct?: number | null;
  avg_latency_ms?: number | null;
  jitter_ms?: number | null;
  signal_trend_7d?: number | null;
  is_stale: boolean;
  calculated_at: string;
};

export type ScoringThreshold = {
  key: string;
  value: number;
  description?: string;
  updated_at: string;
  updated_by?: string | null;
};

export type WorkOrderCandidate = {
  id: string;
  customer_id?: string | null;
  ap_device_id?: string | null;
  tower_id?: string | null;
  source_score_id?: string | null;
  diagnosis: string;
  recommended_action: string;
  severity: Severity;
  reasons: string[];
  status: "open" | "dismissed" | "promoted";
  notes?: string | null;
  promoted_work_order_id?: string | null;
  created_at: string;
  updated_at: string;
};

export type APHealthRow = {
  ap_device_id: string;
  ap_score: number;
  severity: Severity;
  total_customers: number;
  critical_customers: number;
  warning_customers: number;
  healthy_customers: number;
  degradation_ratio: number;
  is_ap_wide_interference: boolean;
  reasons: string[];
  calculated_at: string;
};

export type TowerRiskRow = {
  tower_id: string;
  risk_score: number;
  severity: Severity;
  reasons: string[];
  calculated_at: string;
};
