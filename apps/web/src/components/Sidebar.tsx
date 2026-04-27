import Link from "next/link";

const NAV: { href: string; label: string }[] = [
  { href: "/", label: "Operasyon Paneli" },
  { href: "/musteriler", label: "Sorunlu Müşteriler" },
  { href: "/kuleler", label: "Kuleler" },
  { href: "/linkler", label: "Linkler" },
  { href: "/cihazlar", label: "Cihazlar" },
  { href: "/planli-kontroller", label: "Planlı Kontroller" },
  { href: "/frekans-onerileri", label: "Frekans Önerileri" },
  { href: "/raporlar", label: "Raporlar" },
  { href: "/job-runs", label: "İş Yürütmeleri" },
  { href: "/ap-client-tests", label: "AP → Client Testleri" },
  { href: "/is-emirleri", label: "İş Emirleri" },
  { href: "/ayarlar", label: "Ayarlar" }
];

export function Sidebar() {
  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        WISP Ops Center
        <small>Faz 6 · skor + sorunlu müşteri</small>
      </div>
      {NAV.map((item) => (
        <Link key={item.href} href={item.href} className="sidebar-link">
          {item.label}
        </Link>
      ))}
    </aside>
  );
}
