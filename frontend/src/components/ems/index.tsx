import { useEffect, useRef, type ComponentProps, type ReactNode } from "react"
import { createPortal } from "react-dom"
import { IonBadge, IonButton, IonCard, IonCardContent, IonCardHeader, IonCardSubtitle, IonCardTitle, IonInput, IonSelect, IonSelectOption } from "@ionic/react"

export type EmsTone = "success" | "warning" | "danger" | "info" | "neutral" | "busy"
export const emsToneColor: Record<EmsTone, string> = { success: "success", warning: "warning", danger: "danger", info: "tertiary", neutral: "medium", busy: "secondary" }

export function EmsCard({ children, className = "", variant = "default" }: { children: ReactNode; className?: string; variant?: "default" | "accent" | "subtle" }) {
  return <IonCard className={`ems-card ems-card-${variant} ${className}`}>{children}</IonCard>
}

export function EmsSectionHeader({ kicker, title, description, status, actions }: { kicker?: string; title: string; description?: string; status?: ReactNode; actions?: ReactNode }) {
  return <div className="flex flex-wrap items-start justify-between gap-3">
    <div className="min-w-0">{kicker && <span className="section-kicker">{kicker}</span>}<h3 className="mb-1 text-lg font-semibold text-ems-ink">{title}</h3>{description && <p className="m-0 text-sm leading-5 text-ems-muted">{description}</p>}</div>
    {(status || actions) && <div className="flex shrink-0 flex-wrap items-center gap-2">{status}{actions}</div>}
  </div>
}

export function EmsField({ label, hint, error, children, className = "" }: { label: string; hint?: string; error?: string; children: ReactNode; className?: string }) {
  return <label className={`ems-field ${className}`}>
    <span className="ems-field-label">{label}</span>
    {children}
    {error ? <span className="ems-field-error">{error}</span> : hint ? <span className="ems-field-hint">{hint}</span> : null}
  </label>
}

export function EmsInput({ label, hint, error, className = "", ...props }: ComponentProps<typeof IonInput> & { label?: string; hint?: string; error?: string }) {
  const input = <IonInput className={`ems-input ${className}`} {...props} />
  return label ? <EmsField label={label} hint={hint} error={error}>{input}</EmsField> : input
}

export function EmsSelect({ label, hint, error, className = "", children, ...props }: ComponentProps<typeof IonSelect> & { label?: string; hint?: string; error?: string }) {
  const select = <IonSelect className={`ems-input ${className}`} {...props}>{children}</IonSelect>
  return label ? <EmsField label={label} hint={hint} error={error}>{select}</EmsField> : select
}
export { IonSelectOption as EmsSelectOption }

export function EmsButton({ children, variant = "primary", busy, busyLabel, ...props }: ComponentProps<typeof IonButton> & { variant?: "primary" | "secondary" | "quiet" | "danger"; busy?: boolean; busyLabel?: string }) {
  const fill = variant === "primary" ? "solid" : variant === "quiet" ? "clear" : "outline"
  const color = variant === "danger" ? "danger" : "primary"
  return <IonButton fill={fill} color={color} className={`ems-button ems-button-${variant}`} disabled={busy || props.disabled} {...props}>{busy ? busyLabel || "处理中…" : children}</IonButton>
}

export function StatusBadge({ children, tone = "neutral", dot = true }: { children: ReactNode; tone?: EmsTone; dot?: boolean }) {
  return <IonBadge color={emsToneColor[tone]} className={`ems-status ems-status-${tone}`}>{dot && <span className="ems-status-dot" aria-hidden="true" />}{children}</IonBadge>
}

export function MetricStrip({ items }: { items: { label: string; value: ReactNode; note?: string; tone?: EmsTone; onClick?: () => void }[] }) {
  return <div className="ems-metric-strip">{items.map((item) => <div className={`ems-metric ${item.tone ? `ems-metric-${item.tone}` : ""}`} key={item.label} onClick={item.onClick} role={item.onClick ? "button" : undefined} tabIndex={item.onClick ? 0 : undefined}>
    <span className="ems-metric-label">{item.label}</span><strong className="ems-metric-value">{item.value}</strong>{item.note && <span className="ems-metric-note">{item.note}</span>}
  </div>)}</div>
}

export function FilterToolbar({ children, trailing }: { children: ReactNode; trailing?: ReactNode }) {
  return <div className="flex flex-wrap items-end gap-3"><div className="flex min-w-0 flex-1 flex-wrap items-end gap-3">{children}</div>{trailing && <div className="flex shrink-0 flex-wrap items-center gap-2">{trailing}</div>}</div>
}

export function SegmentedTabs<T extends string>({ value, options, onChange, ariaLabel }: { value: T; options: { value: T; label: string }[]; onChange: (value: T) => void; ariaLabel: string }) {
  return <div className="ems-segments" role="tablist" aria-label={ariaLabel}>{options.map((option) => <button key={option.value} role="tab" aria-selected={value === option.value} className={`ems-segment ${value === option.value ? "active" : ""}`} onClick={() => onChange(option.value)}>{option.label}</button>)}</div>
}

export function EmptyState({ title, description, action }: { title: string; description: string; action?: ReactNode }) {
  return <div className="ems-empty"><strong>{title}</strong><p>{description}</p>{action}</div>
}

export function InlineNotice({ children, tone = "info" }: { children: ReactNode; tone?: EmsTone }) {
  return <div className={`ems-notice ems-notice-${tone}`} role={tone === "danger" ? "alert" : "status"}>{children}</div>
}

export function BatchActionBar({ count, children, trailing }: { count?: number; children: ReactNode; trailing?: ReactNode }) {
  return <div className="ems-batch-bar"><div className="flex flex-wrap items-center gap-2">{count !== undefined && <span className="text-xs text-ems-muted">已选 {count} 项</span>}{children}</div>{trailing && <div className="flex flex-wrap items-center gap-2">{trailing}</div>}</div>
}

export function EmsOverlay({ open, title, children, actions, onClose, variant = "dialog", busy = false }: { open: boolean; title: string; children: ReactNode; actions?: ReactNode; onClose: () => void; variant?: "dialog" | "drawer"; busy?: boolean }) {
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    if (!open) return
    const previous = document.activeElement as HTMLElement | null
    const oldOverflow = document.body.style.overflow
    document.body.style.overflow = "hidden"
    const root = ref.current
    const focusable = () => Array.from(root?.querySelectorAll<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])') || []).filter((el) => !el.hasAttribute("disabled"))
    focusable()[0]?.focus()
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) onClose()
      if (event.key !== "Tab") return
      const items = focusable()
      if (!items.length) return
      const first = items[0], last = items[items.length - 1]
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
    }
    document.addEventListener("keydown", keydown)
    return () => { document.body.style.overflow = oldOverflow; document.removeEventListener("keydown", keydown); previous?.focus() }
  }, [open, busy, onClose])
  if (!open) return null
  return createPortal(<div className="ems-overlay-backdrop" onClick={() => { if (!busy) onClose() }}>
    <section ref={ref} className={`ems-overlay ems-overlay-${variant}`} role="dialog" aria-modal="true" aria-label={title} onClick={(event) => event.stopPropagation()}>
      <div className="ems-overlay-head"><span className="section-kicker">受控操作</span><h3>{title}</h3></div>
      <div className="ems-overlay-body">{children}</div>
      {actions && <div className="ems-overlay-actions">{actions}</div>}
    </section>
  </div>, document.body)
}

export type EmsColumn<T> = { key: string; label: string; render: (row: T) => ReactNode; className?: string }

export function EmsRecordList<T>({ rows, columns, keyOf, empty, mobileCard, onRowClick }: { rows: T[]; columns: EmsColumn<T>[]; keyOf: (row: T) => string; empty: ReactNode; mobileCard?: (row: T) => ReactNode; onRowClick?: (row: T) => void }) {
  if (!rows.length) return <>{empty}</>
  return <div className="ems-records">
    <div className="hidden overflow-x-auto md:block"><table className="ems-table"><thead><tr>{columns.map((column) => <th key={column.key}>{column.label}</th>)}</tr></thead><tbody>{rows.map((row) => <tr key={keyOf(row)} onClick={() => onRowClick?.(row)} className={onRowClick ? "cursor-pointer" : ""}>{columns.map((column) => <td key={column.key} className={column.className}>{column.render(row)}</td>)}</tr>)}</tbody></table></div>
    <div className="grid gap-2 md:hidden">{rows.map((row) => <div className="ems-record-card" key={keyOf(row)} onClick={() => onRowClick?.(row)} role={onRowClick ? "button" : undefined} tabIndex={onRowClick ? 0 : undefined}>{mobileCard ? mobileCard(row) : columns.map((column) => <div key={column.key}><span className="ems-record-label">{column.label}</span>{column.render(row)}</div>)}</div>)}</div>
  </div>
}
