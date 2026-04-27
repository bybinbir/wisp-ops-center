"use client";
import { ReactNode } from "react";

type LabelProps = { label: string; children: ReactNode; help?: string };

export function Field({ label, children, help }: LabelProps) {
  return (
    <label
      style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 13 }}
    >
      <span style={{ color: "var(--text-dim)", fontWeight: 600 }}>{label}</span>
      {children}
      {help ? (
        <span style={{ color: "var(--text-dim)", fontSize: 12 }}>{help}</span>
      ) : null}
    </label>
  );
}

const inputBase: React.CSSProperties = {
  background: "var(--panel-2)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  padding: "6px 10px",
  color: "var(--text)",
  fontSize: 13
};

export function TextInput(
  props: React.InputHTMLAttributes<HTMLInputElement>
) {
  return <input {...props} style={{ ...inputBase, ...props.style }} />;
}

export function Select(
  props: React.SelectHTMLAttributes<HTMLSelectElement>
) {
  return <select {...props} style={{ ...inputBase, ...props.style }} />;
}

export function TextArea(
  props: React.TextareaHTMLAttributes<HTMLTextAreaElement>
) {
  return (
    <textarea
      rows={3}
      {...props}
      style={{ ...inputBase, ...props.style, resize: "vertical" }}
    />
  );
}
