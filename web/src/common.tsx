// SPDX-License-Identifier: AGPL-3.0-only
import {
  useEffect,
  useState,
  useId,
  isValidElement,
  cloneElement,
  type ReactNode,
} from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import { Button } from "./components/ui/button";
export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  const id = useId();
  const control = isValidElement<{ id?: string; "aria-describedby"?: string }>(
    children,
  )
    ? cloneElement(children, {
        id,
        "aria-describedby": hint ? `${id}-hint` : undefined,
      })
    : children;
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {control}
      {hint && <small id={`${id}-hint`}>{hint}</small>}
    </div>
  );
}
export function ErrorMessage({ error }: { error: unknown }) {
  return error ? (
    <div className="error" role="alert">
      <AlertCircle size={16} />
      <span>{error instanceof Error ? error.message : String(error)}</span>
    </div>
  ) : null;
}
export function Loading() {
  return (
    <div className="empty" role="status">
      <Loader2 className="animate-spin" />
      Loading configuration…
    </div>
  );
}
export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}
export function Enabled({ value }: { value: boolean }) {
  return (
    <span className={`status ${value ? "enabled" : "disabled"}`}>
      <span className="dot" />
      {value ? "Enabled" : "Disabled"}
    </span>
  );
}
export function ConfirmButton({
  children,
  onConfirm,
  label,
}: {
  children: ReactNode;
  onConfirm: () => Promise<void>;
  label: string;
}) {
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  return (
    <>
      <Button variant="ghost" size="sm" onClick={() => setConfirm(true)}>
        {children}
      </Button>
      {confirm && (
        <div className="modal-backdrop">
          <section
            className="modal"
            role="alertdialog"
            aria-modal="true"
            aria-label={label}
          >
            <h2>{label}</h2>
            <p>This changes who can access this tenant.</p>
            <ErrorMessage error={error} />
            <div className="actions">
              <Button
                autoFocus
                variant="outline"
                onClick={() => setConfirm(false)}
                disabled={busy}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                loading={busy}
                onClick={async () => {
                  setBusy(true);
                  setError(null);
                  try {
                    await onConfirm();
                    setConfirm(false);
                  } catch (e) {
                    setError(e);
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                Confirm removal
              </Button>
            </div>
          </section>
        </div>
      )}
    </>
  );
}
export function useLoad<T>(
  load: () => Promise<T>,
  dependencies: unknown[],
  keepPrevious = false,
) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let alive = true;
    if (!keepPrevious) setData(undefined);
    setError(null);
    load().then(
      (v) => {
        if (alive) setData(v);
      },
      (e) => {
        if (alive) setError(e);
      },
    );
    return () => {
      alive = false;
    };
  }, dependencies);
  return { data, error };
}
