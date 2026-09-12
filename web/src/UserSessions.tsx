// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from "react";
import { request } from "./api";
import { Button } from "./components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "./components/ui/card";
import { ConfirmButton, ErrorMessage, useLoad } from "./common";

interface BrowserSession {
  id: string;
  provider: string;
  issued_at: string;
  expires_at: string;
}
interface SessionPage {
  supported: boolean;
  store: string;
  sessions: BrowserSession[];
  has_more: boolean;
}
const pageSize = 50;
const date = (value: string) =>
  value.startsWith("0001-") ? "Not set" : new Date(value).toLocaleString();

export function UserSessions({
  userID,
  offset,
  onOffset,
  reload,
}: {
  userID: string;
  offset: number;
  onOffset: (offset: number) => void;
  reload: number;
}) {
  const [refresh, setRefresh] = useState(0);
  const [message, setMessage] = useState("");
  const path = `/users/${userID}/sessions`;
  const { data, error } = useLoad(
    () => request<SessionPage>(`${path}?limit=${pageSize}&offset=${offset}`),
    [userID, offset, reload, refresh],
  );
  const revoke = async (id?: string) => {
    await request(id ? `${path}/${id}` : path, "DELETE");
    setMessage(id ? "Session revoked." : "All previous sessions revoked.");
    onOffset(0);
    setRefresh((n) => n + 1);
  };
  return (
    <Card className="browser-sessions">
      <CardHeader>
        <CardTitle role="heading" aria-level={2}>
          Browser sessions
        </CardTitle>
        <CardDescription>
          Valid sign-ins for this user's email across all tenants. These do not
          indicate whether someone is currently online.
        </CardDescription>
      </CardHeader>
      <CardContent className="form">
        <div className="actions">
          <Button
            variant="outline"
            onClick={() => {
              setMessage("");
              setRefresh((n) => n + 1);
            }}
          >
            Refresh sessions
          </Button>
          {data?.supported && (
            <ConfirmButton
              label="Revoke all browser sessions?"
              confirmLabel="Revoke all sessions"
              description="All existing browser sessions for this email will stop authenticating. The user can sign in again."
              onConfirm={() => revoke()}
            >
              Revoke all sessions
            </ConfirmButton>
          )}
        </div>
        {message && <p role="status">{message}</p>}
        <ErrorMessage error={error} />
        {!data && !error && <p role="status">Loading sessions…</p>}
        {data && !data.supported && (
          <p>
            {data.store === "disabled"
              ? "Browser sign-in is not configured."
              : "Session listing and revocation require Redis/Valkey. JWT sessions remain valid until they expire."}
          </p>
        )}
        {data?.supported && (
          <>
            <small>
              Older sessions appear after their next use. Revoke all also
              invalidates older sessions that are not listed.
            </small>
            {data.sessions.length ? (
              <ul className="rows">
                {data.sessions.map((s) => (
                  <li key={s.id}>
                    <div>
                      <strong>{s.provider}</strong>
                      <div>Created {date(s.issued_at)}</div>
                      <small>Expires {date(s.expires_at)}</small>
                    </div>
                    <ConfirmButton
                      label="Revoke this browser session?"
                      confirmLabel="Revoke session"
                      description="This browser session will stop authenticating. Other sessions remain valid, and the user can sign in again."
                      onConfirm={() => revoke(s.id)}
                    >
                      Revoke session
                    </ConfirmButton>
                  </li>
                ))}
              </ul>
            ) : (
              <p>No valid sessions on this page.</p>
            )}
            {(offset > 0 || data.has_more) && (
              <div className="pagination">
                <Button
                  variant="outline"
                  disabled={offset === 0}
                  onClick={() => onOffset(Math.max(0, offset - pageSize))}
                >
                  Previous sessions
                </Button>
                <span>Page {Math.floor(offset / pageSize) + 1}</span>
                <Button
                  variant="outline"
                  disabled={!data.has_more || offset + pageSize > 1000000}
                  onClick={() => onOffset(offset + pageSize)}
                >
                  Next sessions
                </Button>
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
