export function SkeletonNotice({ children }: { children?: React.ReactNode }) {
  return (
    <div className="banner">
      <strong>Faz 1 iskelet:</strong>{" "}
      {children ??
        "Bu sayfa veri kaynağına bağlı değil. Faz 2 itibarıyla envanter ve telemetri eklendiğinde gerçek veri burada görünür."}
    </div>
  );
}
