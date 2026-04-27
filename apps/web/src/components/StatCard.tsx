type Props = {
  title: string;
  value: string | number;
  meta?: string;
};

export function StatCard({ title, value, meta }: Props) {
  return (
    <div className="card">
      <p className="card-title">{title}</p>
      <p className="card-value">{value}</p>
      {meta ? <div className="card-meta">{meta}</div> : null}
    </div>
  );
}
