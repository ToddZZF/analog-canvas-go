import { parseProject } from "@icm/project-protocol";
import { describe, expect, it } from "vitest";

import {
  isReservedGalleryId,
  withoutReservedEntries,
} from "../apps/editor/src/gallery-reserved-ids";
import {
  type Harness,
  ORIGIN,
  adminOf,
  cookieHeaders,
  environment,
  makerOf,
  ownerAccountOf,
  route,
  submitOne,
} from "./gallery.test-support";

/** Emails the AuthDO sends from now on: recipient, subject and text. */
function outbox(env: Harness, ok = true) {
  const sent: { to: string[]; subject: string; text: string }[] = [];
  env.authDurable.fetchLike = (async (_input: unknown, init?: RequestInit) => {
    sent.push(JSON.parse(String(init?.body)));
    return ok
      ? Response.json({ id: "email-1" })
      : new Response("down", { status: 503 });
  }) as typeof fetch;
  return sent;
}

async function wall(env: Harness, headers: HeadersInit) {
  const response = await route(
    env,
    new Request(`${ORIGIN}/api/gallery`, { headers }),
  );
  return {
    status: response.status,
    ...((await response.json()) as { entries?: { id: string }[] }),
  };
}

async function sevenCircuits(env: Harness) {
  const admin = await adminOf(env);
  for (let index = 0; index < 7; index += 1)
    await submitOne(env, `Circuit ${index}`, { cookie: admin });
}

const trapFrom = (entries: { id: string }[]) =>
  entries.find((entry) => isReservedGalleryId(entry.id))!;

describe("trap circuits", () => {
  it("mix one into a member's list page, which the client drops, and none into the Owner's or a curator's", async () => {
    const env = environment();
    await sevenCircuits(env);
    const member = await wall(env, cookieHeaders(await makerOf(env)));
    expect(member.entries).toHaveLength(8);
    expect(trapFrom(member.entries!)).toBeDefined();
    expect(withoutReservedEntries(member.entries!)).toHaveLength(7);

    for (const cookie of [await adminOf(env), await ownerAccountOf(env)]) {
      const exempt = await wall(env, cookieHeaders(cookie));
      expect(exempt.entries).toHaveLength(7);
    }
    // The read credential (backups) meets none either.
    expect((await wall(env, {})).entries).toHaveLength(7);
  });

  it("block the member who reads one, keep who it was and email the Owner", async () => {
    const env = environment();
    await sevenCircuits(env);
    const maker = await makerOf(env);
    const trap = trapFrom((await wall(env, cookieHeaders(maker))).entries!);
    const sent = outbox(env);

    const read = await route(
      env,
      new Request(`${ORIGIN}/api/gallery/${trap.id}`, {
        headers: {
          Cookie: maker,
          "cf-connecting-ip": "198.51.100.23",
          "cf-ipcountry": "NL",
          "user-agent": "python-requests/2.32",
        },
      }),
    );
    // An ordinary-looking circuit, so the program reading it does not stop.
    expect(read.status).toBe(200);
    const { projectText } = (await read.json()) as { projectText: string };
    expect(parseProject(projectText).documents.length).toBeGreaterThan(0);

    expect(sent).toHaveLength(1);
    expect(sent[0]!.to).toEqual(["zzhishuai@ethz.ch"]);
    expect(sent[0]!.subject).toContain("maker@example.com");
    expect(sent[0]!.text).toContain("198.51.100.23");
    expect(sent[0]!.text).toContain("python-requests/2.32");

    // Every later Gallery request of the account is refused.
    const after = await wall(env, cookieHeaders(maker));
    expect(after.status).toBe(403);
    expect(after).toMatchObject({ error: "account-blocked" });

    const owner = await ownerAccountOf(env);
    const records = (await (
      await env.authDurable.fetch(
        new Request(`${ORIGIN}/api/auth/blocked`, {
          headers: cookieHeaders(owner),
        }),
      )
    ).json()) as { blocked: Record<string, unknown>[] };
    expect(records.blocked).toEqual([
      expect.objectContaining({
        email: "maker@example.com",
        entryId: trap.id,
        ip: "198.51.100.23",
        country: "NL",
        userAgent: "python-requests/2.32",
        notifiedAt: expect.any(String),
        unblockedAt: null,
      }),
    ]);
    // Nobody else reads or lifts them.
    expect(
      (
        await env.authDurable.fetch(
          new Request(`${ORIGIN}/api/auth/blocked`, {
            headers: cookieHeaders(await adminOf(env)),
          }),
        )
      ).status,
    ).toBe(403);

    const userId = (records.blocked[0] as { userId: string }).userId;
    const lifted = await env.authDurable.fetch(
      new Request(`${ORIGIN}/api/auth/blocked/unblock`, {
        method: "POST",
        headers: { Origin: ORIGIN, Cookie: owner },
        body: JSON.stringify({ userId }),
      }),
    );
    expect(await lifted.json()).toEqual({ unblocked: true });
    expect((await wall(env, cookieHeaders(maker))).status).toBe(200);
  });

  it("answer a curator as a missing circuit and block nobody", async () => {
    const env = environment();
    await sevenCircuits(env);
    const trap = trapFrom(
      (await wall(env, cookieHeaders(await makerOf(env)))).entries!,
    );
    const admin = await adminOf(env);
    const sent = outbox(env);
    const read = await route(
      env,
      new Request(`${ORIGIN}/api/gallery/${trap.id}`, {
        headers: cookieHeaders(admin),
      }),
    );
    expect(read.status).toBe(404);
    expect(sent).toHaveLength(0);
    expect((await wall(env, cookieHeaders(admin))).status).toBe(200);
  });

  it("send the Owner's notice again on the next tick when the first fails", async () => {
    const env = environment();
    await sevenCircuits(env);
    const maker = await makerOf(env);
    const trap = trapFrom((await wall(env, cookieHeaders(maker))).entries!);
    outbox(env, false);
    await route(
      env,
      new Request(`${ORIGIN}/api/gallery/${trap.id}/preview.svg`, {
        headers: cookieHeaders(maker),
      }),
    );
    expect((await wall(env, cookieHeaders(maker))).status).toBe(403);
    const sent = outbox(env);
    const retried = await env.authDurable.fetch(
      new Request("https://auth/internal/block-notices", { method: "POST" }),
    );
    expect(await retried.json()).toEqual({ sent: 1 });
    expect(sent[0]!.subject).toContain("maker@example.com");
  });
});
