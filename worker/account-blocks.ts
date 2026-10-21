// Accounts blocked from the Gallery for collecting it in bulk: who, from
// where, when, and which trap circuit gave them away. A block holds by the
// account and by its email, so a new account on the same address stays
// blocked. Records outlive the account and an unblock: they are the Owner's
// evidence (docs/specs/community-gallery.md#crawlers-ai-and-floods).

import type { SqlStorage } from "./gallery-store";

export const ACCOUNT_BLOCKS_TABLE = `
  CREATE TABLE IF NOT EXISTS account_blocks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    email TEXT,
    display_name TEXT NOT NULL,
    provider TEXT NOT NULL,
    blocked_at TEXT NOT NULL,
    entry_id TEXT NOT NULL,
    path TEXT NOT NULL,
    ip TEXT,
    country TEXT,
    user_agent TEXT,
    notified_at TEXT,
    unblocked_at TEXT
  )
`;

export interface AccountBlockRow {
  id: number;
  user_id: string;
  email: string | null;
  display_name: string;
  provider: string;
  blocked_at: string;
  entry_id: string;
  path: string;
  ip: string | null;
  country: string | null;
  user_agent: string | null;
  notified_at: string | null;
  unblocked_at: string | null;
}

export interface BlockEvidence {
  entryId: string;
  path: string;
  ip: string | null;
  country: string | null;
  userAgent: string | null;
}

export interface BlockedAccount {
  id: string;
  email: string | null;
  displayName: string;
  provider: string;
}

/** Whether an active block names this account or its email. */
export function isBlocked(
  sql: SqlStorage,
  userId: string,
  email: string | null,
): boolean {
  return (
    sql
      .exec(
        `SELECT 1 FROM account_blocks
         WHERE unblocked_at IS NULL
           AND (user_id = ? OR (? IS NOT NULL AND LOWER(email) = LOWER(?)))
         LIMIT 1`,
        userId,
        email,
        email,
      )
      .toArray().length > 0
  );
}

/** Records a block, unless one is already active for the account. */
export function recordBlock(
  sql: SqlStorage,
  account: BlockedAccount,
  evidence: BlockEvidence,
  at: string,
): boolean {
  if (isBlocked(sql, account.id, account.email)) return false;
  sql.exec(
    `INSERT INTO account_blocks(user_id, email, display_name, provider,
       blocked_at, entry_id, path, ip, country, user_agent)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    account.id,
    account.email,
    account.displayName,
    account.provider,
    at,
    evidence.entryId.slice(0, 64),
    evidence.path.slice(0, 512),
    evidence.ip?.slice(0, 64) ?? null,
    evidence.country?.slice(0, 8) ?? null,
    evidence.userAgent?.slice(0, 512) ?? null,
  );
  return true;
}

export function unsentNotices(sql: SqlStorage): AccountBlockRow[] {
  return sql
    .exec<AccountBlockRow>(
      "SELECT * FROM account_blocks WHERE notified_at IS NULL ORDER BY id",
    )
    .toArray();
}

export function markNotified(sql: SqlStorage, id: number, at: string): void {
  sql.exec("UPDATE account_blocks SET notified_at = ? WHERE id = ?", at, id);
}

/** Every block, newest first, for the Owner. */
export function listBlocks(sql: SqlStorage) {
  return sql
    .exec<AccountBlockRow>("SELECT * FROM account_blocks ORDER BY id DESC")
    .toArray()
    .map((row) => ({
      userId: row.user_id,
      email: row.email,
      displayName: row.display_name,
      provider: row.provider,
      blockedAt: row.blocked_at,
      entryId: row.entry_id,
      path: row.path,
      ip: row.ip,
      country: row.country,
      userAgent: row.user_agent,
      notifiedAt: row.notified_at,
      unblockedAt: row.unblocked_at,
    }));
}

/** Lifts an account's active block; its record stays. */
export function unblock(sql: SqlStorage, userId: string, at: string): boolean {
  const active = sql
    .exec(
      "SELECT id FROM account_blocks WHERE user_id = ? AND unblocked_at IS NULL",
      userId,
    )
    .toArray();
  if (active.length === 0) return false;
  sql.exec(
    "UPDATE account_blocks SET unblocked_at = ? WHERE user_id = ? AND unblocked_at IS NULL",
    at,
    userId,
  );
  return true;
}

/** The Owner's email about one block. */
export function blockNotice(row: AccountBlockRow): {
  subject: string;
  text: string;
} {
  const who = row.email ?? `(no email) ${row.display_name}`;
  return {
    subject: `[Analog Canvas] 已封号（批量采集 Gallery）：${who}`,
    text: [
      "一个账号读取了 Gallery 的蜜罐电路（界面上看不到，只有程序批量遍历列表才会读到），已被自动封禁 Gallery 访问。",
      "",
      `邮箱：${row.email ?? "（无）"}`,
      `显示名：${row.display_name}`,
      `登录方式：${row.provider}`,
      `账号 ID：${row.user_id}`,
      `时间（UTC）：${row.blocked_at}`,
      `读取的蜜罐：${row.entry_id}（${row.path}）`,
      `IP：${row.ip ?? "（未知）"}${row.country ? `，国家/地区：${row.country}` : ""}`,
      `User-Agent：${row.user_agent ?? "（无）"}`,
      "",
      "记录永久保存，可在 Owner 账号下查看：GET /api/auth/blocked",
      '误封可解封：POST /api/auth/blocked/unblock {"userId": "…"}',
    ].join("\n"),
  };
}
