"use client";
import { ReactNode } from "react";

export function Toolbar({ children }: { children: ReactNode }) {
  return (
    <div
      style={{
        display: "flex",
        gap: 8,
        marginBottom: 12,
        flexWrap: "wrap",
        alignItems: "center"
      }}
    >
      {children}
    </div>
  );
}

export function Button({
  variant = "primary",
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "danger";
}) {
  const style: React.CSSProperties = {
    border: "1px solid var(--border)",
    background:
      variant === "primary"
        ? "var(--accent)"
        : variant === "danger"
          ? "var(--bad)"
          : "var(--panel-2)",
    color:
      variant === "primary" || variant === "danger"
        ? "#fff"
        : "var(--text)",
    borderRadius: 6,
    padding: "6px 14px",
    cursor: "pointer",
    fontSize: 13,
    fontWeight: 600
  };
  return <button {...props} style={{ ...style, ...props.style }} />;
}
