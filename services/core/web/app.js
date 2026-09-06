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

function setupScrollVideo() {
  const stage = document.querySelector(".video-stage");
  const video = document.getElementById("hero-video");
  const canvas = document.getElementById("hero-canvas");
  if (!stage || !(video instanceof HTMLVideoElement) || !(canvas instanceof HTMLCanvasElement)) return;
  const context = canvas.getContext("2d", { alpha: false });
  if (!context) return;
  let target = 0;
  let progress = 0;
  let frameRequested = false;
  let canvasDisabled = false;
  let seeking = false;
  let cacheReady = false;
  let cacheReleased = false;
  let frames = [];
  let lastFrameIndex = -1;
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");

  const drawCover = (source, sourceWidth, sourceHeight) => {
    const width = window.innerWidth;
    const height = window.innerHeight;
    const scale = Math.max(width / sourceWidth, height / sourceHeight);
    const drawWidth = sourceWidth * scale;
    const drawHeight = sourceHeight * scale;
    context.fillStyle = "#0a0a0a";
    context.fillRect(0, 0, width, height);
    context.drawImage(source, (width - drawWidth) / 2, (height - drawHeight) / 2, drawWidth, drawHeight);
  };

  const resize = () => {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.max(1, Math.round(window.innerWidth * dpr));
    canvas.height = Math.max(1, Math.round(window.innerHeight * dpr));
    context.setTransform(dpr, 0, 0, dpr, 0, 0);
    lastFrameIndex = -1;
  };

  const draw = () => {
    frameRequested = false;
    if (canvasDisabled) return;
    try {
      if (cacheReady && frames.length) {
        const index = Math.min(frames.length - 1, Math.floor(progress * (frames.length - 1)));
        if (index !== lastFrameIndex) {
          drawCover(frames[index], frames[index].width, frames[index].height);
          lastFrameIndex = index;
          stage.classList.add("is-ready");
        }
      } else if (video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
        drawCover(video, video.videoWidth, video.videoHeight);
        stage.classList.add("is-ready");
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === "SecurityError") {
        canvasDisabled = true;
        stage.classList.add("canvas-unavailable");
        return;
      }
      throw error;
    }
  };

  const requestDraw = () => {
    if (!frameRequested) {
      frameRequested = true;
      requestAnimationFrame(draw);
    }
  };

  const tick = () => {
    if (reducedMotion.matches) {
      target = 0;
      progress = 0;
    } else {
      progress = target;
    }
    if (video.readyState >= HTMLMediaElement.HAVE_METADATA && video.duration) {
      const nextTime = progress * Math.max(0, video.duration - .05);
      if (!reducedMotion.matches && !cacheReady && Math.abs(video.currentTime - nextTime) > .04) {
        seeking = true;
        video.currentTime = nextTime;
      }
      if (!canvasDisabled && (cacheReady || video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA)) requestDraw();
    }
    requestAnimationFrame(tick);
  };

  const updateTarget = () => {
    target = Math.min(1, Math.max(0, window.scrollY / Math.max(1, document.documentElement.scrollHeight - window.innerHeight)));
  };

  const extractFrames = async () => {
    await new Promise((resolve) => setTimeout(resolve, 300));
    if (
      cacheReleased ||
      !video.duration ||
      !("createImageBitmap" in window) ||
      !("OffscreenCanvas" in window)
    ) return;
    const offscreen = document.createElement("video");
    offscreen.muted = true;
    offscreen.preload = "auto";
    offscreen.src = video.currentSrc;
    await new Promise((resolve, reject) => {
      offscreen.addEventListener("loadedmetadata", resolve, { once: true });
      offscreen.addEventListener("error", reject, { once: true });
    });
    if (offscreen.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) {
      await new Promise((resolve, reject) => {
        offscreen.addEventListener("loadeddata", resolve, { once: true });
        offscreen.addEventListener("error", reject, { once: true });
      });
    }
    const count = Math.min(45, Math.max(24, Math.ceil(offscreen.duration * 8)));
    const ratio = Math.min(1, 640 / offscreen.videoWidth);
    for (let index = 0; index < count; index += 1) {
      if (cacheReleased) return;
      await new Promise((resolve) => {
        offscreen.addEventListener("seeked", resolve, { once: true });
        offscreen.currentTime = (index / (count - 1)) * Math.max(0, offscreen.duration - .05);
      });
      const bitmap = await createImageBitmap(offscreen);
      if (cacheReleased) {
        bitmap.close();
        return;
      }
      if (ratio < 1) {
        const resized = new OffscreenCanvas(
          Math.max(1, Math.round(offscreen.videoWidth * ratio)),
          Math.max(1, Math.round(offscreen.videoHeight * ratio)),
        );
        const resizedContext = resized.getContext("2d");
        if (!resizedContext) throw new Error("Unable to resize cached video frame");
        resizedContext.drawImage(bitmap, 0, 0, resized.width, resized.height);
        bitmap.close();
        const resizedBitmap = await createImageBitmap(resized);
        if (cacheReleased) {
          resizedBitmap.close();
          return;
        }
        frames.push(resizedBitmap);
      } else {
        frames.push(bitmap);
      }
    }
    cacheReady = true;
    stage.classList.add("cache-ready");
    stage.classList.add("video-ready");
    requestDraw();
  };

  const onLoadedData = () => {
    stage.classList.add("video-ready");
    requestDraw();
    extractFrames().catch(() => {
      frames.forEach((frame) => frame.close());
      frames = [];
    });
  };

  const releaseCache = () => {
    if (cacheReleased) return;
    cacheReleased = true;
    frames.forEach((frame) => frame.close());
    frames = [];
    cacheReady = false;
    lastFrameIndex = -1;
    stage.classList.remove("cache-ready");
  };
  window.addEventListener("pagehide", releaseCache, { once: true });

  video.addEventListener("loadeddata", onLoadedData, { once: true });
  if (video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) onLoadedData();
  video.addEventListener("seeked", () => {
    seeking = false;
    requestDraw();
  });
  window.addEventListener("resize", () => { resize(); requestDraw(); });
  window.addEventListener("scroll", updateTarget, { passive: true });
  resize();
  updateTarget();
  requestAnimationFrame(tick);
}

function setupReveals() {
  const targets = document.querySelectorAll("[data-reveal]");
  if (!("IntersectionObserver" in window)) {
    targets.forEach((target) => target.classList.add("is-visible"));
    return;
  }
  const observer = new IntersectionObserver((entries, currentObserver) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) {
        entry.target.classList.add("is-visible");
        currentObserver.unobserve(entry.target);
      }
    });
  }, { threshold: .15 });
  targets.forEach((target) => observer.observe(target));
}

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
  if (target) {
    target.textContent = `${publicMcpOrigin}/mcp`;
    target.href = `${publicMcpOrigin}/mcp`;
  }
}

async function copyMcpUrl(button) {
  const url = `${publicMcpOrigin}/mcp`;
  try {
    await navigator.clipboard.writeText(url);
    button.textContent = "Copied";
    window.setTimeout(() => { button.textContent = "Copy"; }, 1600);
  } catch {
    button.textContent = "Copy failed";
    window.setTimeout(() => { button.textContent = "Copy"; }, 1600);
  }
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
  setupScrollVideo();
  setupReveals();
  renderTerminal();
  renderMcpUrl();
  document.querySelectorAll("[data-copy-prompt]").forEach((button) => {
    button.addEventListener("click", () => copyPrompt(button));
  });
  document.querySelectorAll("[data-copy-mcp]").forEach((button) => {
    button.addEventListener("click", () => copyMcpUrl(button));
  });
});
