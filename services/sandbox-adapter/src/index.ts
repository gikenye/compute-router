// This Worker exposes the sandbox adapter's internal HTTP API.

import { getSandbox } from "@cloudflare/sandbox";
export { Sandbox } from "@cloudflare/sandbox";

interface Env {
  Sandbox: any;
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

        const sandbox = getSandbox(env.Sandbox as any, session_id);
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
        const sandbox = getSandbox(env.Sandbox, session_id);
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
        // Explicit destruction is preferred; the SDK also reclaims idle
        // sandboxes through its configured inactivity timeout.
        const sandbox: any = getSandbox(env.Sandbox, session_id);
        if (typeof sandbox.destroy === "function") {
          await sandbox.destroy();
        }
        return Response.json({ destroyed: true });
      } catch (err: any) {
        // Cleanup is best effort because the SDK's idle timeout is the
        // fallback when explicit destruction cannot complete.
        return Response.json({ destroyed: false, note: String(err?.message ?? err) });
      }
    }

    return new Response(JSON.stringify({ error: "not found" }), { status: 404 });
  },
};
