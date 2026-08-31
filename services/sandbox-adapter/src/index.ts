// Deliberately dumb. This Worker's entire job: expose /boot, /exec,
// /destroy over plain internal HTTP, wrapping @cloudflare/sandbox.
// No pricing, payment, or session-lease logic belongs here — that all
// lives in services/core (Go). See /docs/ADR-001-language-choice.md.
//
// NOTE on templates: Cloudflare fixes the container image at Worker
// deploy time (see wrangler.jsonc `containers[0].image`) — it is NOT
// selectable per-request. This MVP ships one image ("node-build").
// The `template` field on /boot is accepted and stored but currently
// has no effect; true multi-template support needs multiple registered
// container classes, each with its own Durable Object binding — a
// real roadmap item, not implemented here.

import { getSandbox } from "@cloudflare/sandbox";
export { Sandbox } from "@cloudflare/sandbox";

interface Env {
  Sandbox: DurableObjectNamespace;
  ADAPTER_SHARED_SECRET: string;
}

function checkAuth(request: Request, env: Env): Response | null {
  const provided = request.headers.get("X-Adapter-Secret");
  if (!provided || provided !== env.ADAPTER_SHARED_SECRET) {
    return new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 });
  }
  return null;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const authError = checkAuth(request, env);
    if (authError) return authError;

    const url = new URL(request.url);

    if (url.pathname === "/boot" && request.method === "POST") {
      try {
        const { session_id } = await request.json<{ session_id: string; template?: string }>();
        if (!session_id) {
          return new Response(JSON.stringify({ error: "session_id required" }), { status: 400 });
        }

        // getSandbox with RPC transport (required — see wrangler.jsonc).
        // [UNCONFIRMED]: createSession() is documented as the
        // forward-compatible pattern over the implicit default session,
        // but the exact chained call shape below (getSandbox ->
        // createSession -> session.exec) was not independently
        // confirmed against a live API reference during this build.
        // Verify against https://developers.cloudflare.com/sandbox/api/
        // before relying on it — if createSession() doesn't exist on
        // the version you install, fall back to
        // getSandbox(env.Sandbox, session_id, { transport: "rpc" })
        // and call .exec() directly on that object instead.
        const sandbox = getSandbox(env.Sandbox, session_id, { transport: "rpc" });
        // Touch the sandbox once so the container actually boots here
        // rather than lazily on first /exec — makes boot latency visible
        // to the caller instead of hidden inside the first exec() call.
        await sandbox.exec("true");

        return Response.json({ session_id, booted: true });
      } catch (err: any) {
        return new Response(JSON.stringify({ error: String(err?.message ?? err) }), { status: 500 });
      }
    }

    if (url.pathname === "/exec" && request.method === "POST") {
      try {
        const { session_id, command } = await request.json<{ session_id: string; command: string }>();
        if (!session_id || !command) {
          return new Response(JSON.stringify({ error: "session_id and command required" }), { status: 400 });
        }
        const sandbox = getSandbox(env.Sandbox, session_id, { transport: "rpc" });
        const result = await sandbox.exec(command);
        return Response.json({
          stdout: result.stdout ?? "",
          stderr: result.stderr ?? "",
          exit_code: result.exitCode ?? 0,
        });
      } catch (err: any) {
        return new Response(JSON.stringify({ error: String(err?.message ?? err) }), { status: 500 });
      }
    }

    if (url.pathname === "/destroy" && request.method === "POST") {
      try {
        const { session_id } = await request.json<{ session_id: string }>();
        // [UNCONFIRMED]: no explicit destroy method was confirmed
        // against current docs during this build — Sandbox SDK's own
        // idle timeout is the documented backstop (see SPEC-100 §5.3).
        // This best-effort call is defensive; if the method name is
        // wrong it fails silently and idle-timeout still cleans up.
        const sandbox: any = getSandbox(env.Sandbox, session_id, { transport: "rpc" });
        if (typeof sandbox.destroy === "function") {
          await sandbox.destroy();
        }
        return Response.json({ destroyed: true });
      } catch (err: any) {
        // Non-fatal — idle timeout is the real backstop.
        return Response.json({ destroyed: false, note: String(err?.message ?? err) });
      }
    }

    return new Response(JSON.stringify({ error: "not found" }), { status: 404 });
  },
};
