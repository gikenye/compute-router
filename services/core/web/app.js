const lines = [
  ["$ provision_env()", ""],
  ["→ payment_required", "dim"],
  ["  payment needed · USDC or USDT on Celo", "dim"],
  ["$ agent signs payment_data + retries", ""],
  ["→ sandbox online · session issued", "green"],
  ["$ exec({ command: \"npm test\" })", ""],
  ["→ stdout · exit_code: 0", "green"],
  ["$ release({ session_id })", ""],
  ["→ settled · environment destroyed", "green"],
];

const publicMcpOrigin = "https://mcp.computerouter.wtf";

function renderTerminal() {
  const target = document.getElementById("terminal-body");
  if (!target) return;
  lines.forEach(([text, style], index) => {
    const line = document.createElement("span");
    line.className = `line ${style}`;
    line.textContent = text;
    line.style.animationDelay = `${index * 0.18}s`;
    target.appendChild(line);
  });
}

function renderMcpUrl() {
  const target = document.getElementById("mcp-url");
  if (target) target.textContent = `${publicMcpOrigin}/mcp`;
}

function getAgentPrompt() {
  const mcpUrl = `${publicMcpOrigin}/mcp`;
  return `Connect to the Compute Router MCP server at ${mcpUrl}.

Use your provider's MCP integration or Streamable HTTP client to initialize a session at that URL. Read the agent card at ${publicMcpOrigin}/.well-known/agent-card.json for the available tools and payment instructions.

When I ask for a compute environment, use provision_env first, then exec for commands in the returned session, and release when finished. Follow the server's payment requirements and report any payment or session errors clearly.`;
}

async function copyPrompt(button) {
  const label = button.querySelector("[data-copy-label]");
  try {
    const prompt = getAgentPrompt();
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(prompt);
    } else {
      const input = document.createElement("textarea");
      input.value = prompt;
      input.setAttribute("readonly", "");
      input.style.position = "fixed";
      input.style.opacity = "0";
      document.body.appendChild(input);
      input.select();
      if (!document.execCommand("copy")) throw new Error("Clipboard copy was rejected");
      input.remove();
    }
    if (label) label.textContent = "Prompt copied!";
    button.dataset.copied = "true";
    window.setTimeout(() => {
      if (label) label.textContent = "Copy prompt";
      delete button.dataset.copied;
    }, 2200);
  } catch (error) {
    if (label) label.textContent = "Copy failed";
    window.setTimeout(() => {
      if (label) label.textContent = "Copy prompt";
    }, 2200);
  }
}

document.addEventListener("DOMContentLoaded", () => {
  renderTerminal();
  renderMcpUrl();
  document.querySelectorAll("[data-copy-prompt]").forEach((button) => {
    button.addEventListener("click", () => copyPrompt(button));
  });
});
