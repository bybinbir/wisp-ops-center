"use client";
import { useEffect, useState } from "react";
import { api, ApiError } from "@/lib/api";
import { PageHeader } from "@/components/PageHeader";

type JobRun = {
  id: string;
  scheduled_check_id?: string;
  job_type: string;
  scope_type?: string;
  status: string;
  started_at: string;
  finished_at?: string;
  duration_ms?: number;
  error_code?: string;
  error_message?: string;
  summary: Record<string, unknown>;
};

export default function JobRunsPage() {
  const [list, setList] = useState<JobRun[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.get<{ data: JobRun[] }>("/api/v1/job-runs")
      .then((r) => setList(r.data ?? []))
      .catch((e) => setError(e instanceof ApiError && e.status === 503 ? "Veritabanı bağlı değil." : String(e)));
  }, []);

  return (
    <div>
      <PageHeader title="İş Yürütmeleri" subtitle="Son 200 job_runs kaydı; scheduler ve run-now çağrıları." />
      {error ? <div className="banner">{error}</div> : null}
      <table>
        <thead>
          <tr>
            <th>Zaman</th><th>Job</th><th>Scope</th><th>Status</th><th>Süre</th><th>Hata</th>
          </tr>
        </thead>
        <tbody>
          {list && list.length === 0 ? (
            <tr><td colSpan={6} className="empty">Henüz job_run yok.</td></tr>
          ) : null}
          {(list ?? []).map((j) => (
            <tr key={j.id}>
              <td>{new Date(j.started_at).toLocaleString("tr-TR")}</td>
              <td>{j.job_type}</td>
              <td>{j.scope_type ?? "—"}</td>
              <td>
                <span className={`badge ${j.status === "success" ? "good" : j.status === "queued" ? "warn" : j.status === "running" ? "warn" : "bad"}`}>
                  {j.status}
                </span>
              </td>
              <td>{j.duration_ms != null ? `${j.duration_ms} ms` : "—"}</td>
              <td>{j.error_message ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
