// Metapi Desktop — Electron main process.
//
// A thin wrapper around the Go admin server:
//   - spawns/manages the `metapi` Go binary (cwd = this app's directory),
//   - opens a BrowserWindow that loads the Go admin UI at http://127.0.0.1:4000,
//   - shows a "waiting for server" screen and retries every 2s until the server
//     is ready, then loads the real UI,
//   - adds a system-tray icon with Start/Stop/Open Admin/Quit + auto-start toggle,
//   - surfaces native OS notifications by polling the server's notifications
//     endpoint (best-effort: the admin API is auth-gated, so polling is opt-in
//     via METAPI_DESKTOP_NOTIFY_TOKEN; 401/404 are handled silently).
//
// This is intentionally minimal — it does NOT reimplement the original TS
// Electron app's full feature set. See electron/README.md.

const {
  app,
  BrowserWindow,
  Tray,
  Menu,
  nativeImage,
  shell,
  Notification,
  ipcMain,
} = require('electron');
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');

// --- Configuration ----------------------------------------------------------

const PKG = require('./package.json');
const CFG = (PKG.config && PKG.config.metapi) || {};
const ADMIN_URL = CFG.adminUrl || 'http://127.0.0.1:4000';
const HEALTH_PATH = CFG.healthPath || '/ready';
const NOTIFICATIONS_PATH = CFG.notificationsPath || '/api/admin/notifications';
const POLL_INTERVAL_MS = CFG.pollIntervalMs || 2000;

// Sub-directory this script lives in (= the electron/ project dir, both in dev
// and after packaging via electron-packager's `--resourcesPath` layout).
const APP_DIR = __dirname;

// --- Runtime state (module-scoped; single-window app) -----------------------

let mainWindow = null;
let tray = null;
let serverProcess = null;
let isStartingServer = false;
let notificationTimer = null;
// IDs of notifications already shown natively, to avoid duplicates.
const shownNotificationIds = new Set();

// --- Go binary resolution ---------------------------------------------------

function binaryName() {
  return process.platform === 'win32' ? 'metapi.exe' : 'metapi';
}

// Locate the Go binary. Search order:
//   1. METAPI_BINARY env var (explicit override)
//   2. packaged app: process.resourcesPath/<binary>
//   3. dev: <APP_DIR>/<binary>
function resolveBinaryPath() {
  const name = binaryName();
  const candidates = [];
  if (process.env.METAPI_BINARY) candidates.push(process.env.METAPI_BINARY);
  if (app.isPackaged) candidates.push(path.join(process.resourcesPath, name));
  candidates.push(path.join(APP_DIR, name));
  for (const candidate of candidates) {
    try {
      if (fs.existsSync(candidate)) return candidate;
    } catch {
      // ignore stat errors (e.g. permission), try next candidate
    }
  }
  // Return the most likely default so the caller can report a clear error.
  return candidates[candidates.length - 1];
}

function binaryExists() {
  try {
    return fs.existsSync(resolveBinaryPath());
  } catch {
    return false;
  }
}

// --- Server process lifecycle ----------------------------------------------

// Returns a writable stream that captures the Go binary's stdout/stderr.
// In dev the lines are also echoed to the terminal console.
function openServerLogStream() {
  const logsDir = app.getPath('logs');
  try {
    fs.mkdirSync(logsDir, { recursive: true });
  } catch {
    // logs dir creation is best-effort; fall back to /dev/null-like behavior
  }
  const logFile = path.join(logsDir, 'metapi-server.log');
  const stream = fs.createWriteStream(logFile, { flags: 'a' });
  stream.write(`\n==== metapi server log ${new Date().toISOString()} ====\n`);
  return { stream, logFile };
}

function startServer() {
  if (serverProcess || isStartingServer) {
    return;
  }
  if (!binaryExists()) {
    const resolved = resolveBinaryPath();
    new Notification({
      title: 'Metapi Desktop',
      body: `Go binary not found at ${resolved}. Build it with: make electron-build`,
    }).show();
    return;
  }

  isStartingServer = true;
  const { stream, logFile } = openServerLogStream();

  try {
    serverProcess = spawn(resolveBinaryPath(), [], {
      cwd: APP_DIR,
      env: { ...process.env },
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true,
    });
  } catch (err) {
    isStartingServer = false;
    stream.end(`failed to spawn metapi: ${err.stack || err}\n`);
    new Notification({
      title: 'Metapi Desktop',
      body: `Failed to start server: ${err.message}`,
    }).show();
    return;
  }

  serverProcess.stdout.on('data', (chunk) => {
    stream.write(chunk);
    if (!app.isPackaged) process.stdout.write(chunk);
  });
  serverProcess.stderr.on('data', (chunk) => {
    stream.write(chunk);
    if (!app.isPackaged) process.stderr.write(chunk);
  });

  serverProcess.on('error', (err) => {
    isStartingServer = false;
    stream.write(`server process error: ${err.stack || err}\n`);
    new Notification({
      title: 'Metapi Desktop',
      body: `Server process error: ${err.message}`,
    }).show();
  });

  serverProcess.on('exit', (code, signal) => {
    isStartingServer = false;
    const prev = serverProcess;
    serverProcess = null;
    stream.write(`metapi exited (code=${code}, signal=${signal})\n`);
    stream.end();
    // Surface unexpected exits (Quit/Stop set the flag before killing).
    if (prev && !prev._stoppedByDesktop) {
      new Notification({
        title: 'Metapi Desktop',
        body: `Server stopped (code ${code}). Use "Start Server" to relaunch.`,
      }).show();
    }
    updateTrayMenu();
  });

  isStartingServer = false;
  updateTrayMenu();
}

function stopServer() {
  if (!serverProcess) return;
  serverProcess._stoppedByDesktop = true;
  // SIGTERM is cross-platform-ish: on Windows, Node translates this to a hard
  // kill of the process tree via taskkill, which is acceptable for a stop action.
  serverProcess.kill('SIGTERM');
  // Guard against hangs: force-kill after a short grace period.
  const proc = serverProcess;
  setTimeout(() => {
    if (proc && !proc.killed) {
      try {
        proc.kill('SIGKILL');
      } catch {
        // already gone — nothing to do
      }
    }
  }, 3000);
  updateTrayMenu();
}

// --- Window management ------------------------------------------------------

function waitingScreenHtml(reason) {
  return `data:text/html;charset=utf-8,${encodeURIComponent(`<!doctype html>
<html><head><meta charset="utf-8"><title>Metapi Desktop</title>
<style>
  html,body{height:100%;margin:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#0f172a;color:#e2e8f0}
  main{height:100%;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:16px;text-align:center}
  .dot{width:14px;height:14px;border-radius:50%;background:#38bdf8;animation:p 1s ease-in-out infinite}
  @keyframes p{0%,100%{opacity:.3}50%{opacity:1}}
  h1{font-size:18px;font-weight:600;margin:0}
  p{font-size:13px;color:#94a3b8;margin:0;max-width:340px;line-height:1.5}
</style></head>
<body><main>
  <div class="dot"></div>
  <h1>Waiting for Metapi server…</h1>
  <p>${reason || 'The Go server is starting. The admin UI will load automatically.'}</p>
</main></body></html>`)}`;
}

function createWindow() {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.focus();
    return;
  }
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 960,
    minHeight: 600,
    title: 'Metapi Desktop',
    backgroundColor: '#0f172a',
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(APP_DIR, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false, // preload needs ipcRenderer; contextIsolation stays on
    },
  });

  // Open external http(s) links in the user's default browser, not a new window.
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:\/\//.test(url)) {
      shell.openExternal(url);
      return { action: 'deny' };
    }
    return { action: 'allow' };
  });

  // If the SPA later fails to load (e.g. server restarted mid-session), fall
  // back to the waiting screen and retry.
  mainWindow.webContents.on('did-fail-load', () => {
    if (!mainWindow || mainWindow.isDestroyed()) return;
    mainWindow.loadURL(waitingScreenHtml('Lost connection to the server. Reconnecting…'));
    setTimeout(loadAdminUI, POLL_INTERVAL_MS);
  });

  loadAdminUI();
}

// Health-check the Go server. The Go server already exposes a dedicated,
// public (auth-free) desktop health endpoint at /api/desktop/health that
// returns {"status":"ok"} — we use that for parity with the existing stub.
// Once reachable, load the real admin UI.
async function loadAdminUI() {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  mainWindow.loadURL(waitingScreenHtml());
  // Poll until the server answers, then swap in the real UI.
  const poll = async () => {
    const reachable = await serverReady();
    if (reachable) {
      if (mainWindow && !mainWindow.isDestroyed()) {
        mainWindow.loadURL(ADMIN_URL);
      }
      return;
    }
    setTimeout(poll, POLL_INTERVAL_MS);
  };
  poll();
}

function serverReady() {
  return new Promise((resolve) => {
    const url = new URL(ADMIN_URL);
    const req = http.get(
      {
        hostname: url.hostname,
        port: url.port,
        path: HEALTH_PATH,
        timeout: POLL_INTERVAL_MS,
      },
      (res) => {
        res.resume();
        resolve(res.statusCode !== undefined && res.statusCode < 500);
      },
    );
    req.on('error', () => resolve(false));
    req.on('timeout', () => {
      req.destroy();
      resolve(false);
    });
  });
}

// --- System tray ------------------------------------------------------------

function trayIconImage() {
  // Prefer a monochrome template image on macOS; fall back to the full-color
  // icon elsewhere (and if the template file is missing).
  const iconFile = path.join(APP_DIR, 'tray-icon.png');
  try {
    const image = nativeImage.createFromPath(iconFile);
    if (!image.isEmpty()) {
      if (process.platform === 'darwin') image.setTemplateImage(true);
      return image;
    }
  } catch {
    // fall through to empty image
  }
  return nativeImage.createEmpty();
}

function buildTrayMenu() {
  const serverRunning = !!serverProcess && !serverProcess.killed;
  const autoStartOn = app.getLoginItemSettings().openAtLogin;
  return Menu.buildFromTemplate([
    { label: 'Metapi Desktop', enabled: false },
    { type: 'separator' },
    {
      label: 'Open Admin',
      click: () => {
        if (!mainWindow || mainWindow.isDestroyed()) {
          createWindow();
        } else {
          if (mainWindow.isMinimized()) mainWindow.restore();
          mainWindow.show();
          mainWindow.focus();
        }
      },
    },
    {
      label: 'Start Server',
      enabled: !serverRunning,
      click: () => startServer(),
    },
    {
      label: 'Stop Server',
      enabled: serverRunning,
      click: () => stopServer(),
    },
    { type: 'separator' },
    {
      label: 'Launch at login',
      type: 'checkbox',
      checked: autoStartOn,
      click: (item) => app.setLoginItemSettings({ openAtLogin: item.checked }),
    },
    { type: 'separator' },
    {
      label: 'Quit',
      click: () => quitApp(),
    },
  ]);
}

function updateTrayMenu() {
  if (!tray) return;
  tray.setContextMenu(buildTrayMenu());
}

function createTray() {
  tray = new Tray(trayIconImage());
  tray.setToolTip('Metapi Desktop');
  tray.setContextMenu(buildTrayMenu());
  // Click the tray icon to bring the window forward (common desktop idiom).
  tray.on('click', () => {
    if (!mainWindow || mainWindow.isDestroyed()) {
      createWindow();
    } else {
      mainWindow.show();
      mainWindow.focus();
    }
  });
}

// --- Native notifications (best-effort polling) -----------------------------

// The admin API is auth-gated. We only poll for desktop notifications when an
// operator provides METAPI_DESKTOP_NOTIFY_TOKEN (bearer), to avoid spamming the
// server with 401s. 404 means the endpoint isn't implemented yet — we back off
// silently and log once.
function startNotificationPolling() {
  if (notificationTimer) clearInterval(notificationTimer);
  notificationTimer = setInterval(pollNotifications, 30 * 1000);
  // Fire once shortly after startup so the first check is quick.
  setTimeout(pollNotifications, 5 * 1000);
}

function stopNotificationPolling() {
  if (notificationTimer) {
    clearInterval(notificationTimer);
    notificationTimer = null;
  }
}

let notificationsEndpointMissing = false;

function pollNotifications() {
  const token = process.env.METAPI_DESKTOP_NOTIFY_TOKEN;
  if (!token || notificationsEndpointMissing) return;
  const url = new URL(ADMIN_URL);
  const req = http.get(
    {
      hostname: url.hostname,
      port: url.port,
      path: NOTIFICATIONS_PATH,
      headers: { Authorization: `Bearer ${token}` },
      timeout: 5000,
    },
    (res) => {
      res.resume();
      if (res.statusCode === 404) {
        notificationsEndpointMissing = true; // endpoint not implemented; stop polling
        return;
      }
      if (res.statusCode !== 200) return;
      let body = '';
      res.on('data', (chunk) => (body += chunk));
      res.on('end', () => {
        try {
          const parsed = JSON.parse(body);
          const items = Array.isArray(parsed) ? parsed : parsed.items || parsed.notifications || [];
          for (const item of items) {
            const id = String(item.id || item._id || `${item.title}:${item.body}`);
            if (shownNotificationIds.has(id)) continue;
            shownNotificationIds.add(id);
            const notification = new Notification({
              title: item.title || 'Metapi',
              body: typeof item.body === 'string' ? item.body : JSON.stringify(item.body || ''),
            });
            notification.on('click', () => {
              if (item.url) shell.openExternal(item.url);
            });
            notification.show();
            // Forward to the renderer so the SPA can mirror/ack it.
            if (mainWindow && !mainWindow.isDestroyed()) {
              mainWindow.webContents.send('metapi:native-notification', { id, title: item.title, body: item.body });
            }
          }
        } catch {
          // non-JSON or unexpected shape — ignore this round
        }
      });
    },
  );
  req.on('error', () => {});
  req.on('timeout', () => req.destroy());
}

// Ack handler forwarded from the preload/renderer (no-op stub for now; future
// use: clear the server-side notification queue).
ipcMain.on('metapi:ack-notification', (_event, _id) => {
  // Intentional no-op: ack is a forward-compatible hook.
});

// --- App lifecycle ----------------------------------------------------------

function quitApp() {
  stopNotificationPolling();
  stopServer();
  // Give the server a brief moment to flush logs on SIGTERM.
  setTimeout(() => {
    if (tray) tray.destroy();
    app.quit();
  }, 200);
}

// Single-instance lock: don't allow multiple desktop instances racing the port.
const gotLock = app.requestSingleInstanceLock();
if (!gotLock) {
  app.quit();
} else {
  app.on('second-instance', () => {
    // Someone tried to launch a second copy — focus the existing window.
    if (mainWindow && !mainWindow.isDestroyed()) {
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.show();
      mainWindow.focus();
    }
  });

  app.whenReady().then(() => {
    createTray();
    createWindow();
    // Boot the server automatically. The window is already showing the waiting
    // screen, which will flip to the admin UI once the server answers /ready.
    startServer();
    startNotificationPolling();
  });

  // macOS: re-open the window when the Dock icon is clicked and no window exists.
  app.on('activate', () => {
    if (!mainWindow || mainWindow.isDestroyed()) createWindow();
  });

  app.on('window-all-closed', () => {
    // On macOS keep the app alive in the tray; elsewhere quit when all windows
    // close (the tray still works to reopen via "Open Admin").
    if (process.platform !== 'darwin') {
      // Keep the process alive so the tray + server keep running even without a
      // window. Use `quitApp()` from the tray to fully exit.
    }
  });

  app.on('before-quit', () => {
    stopNotificationPolling();
    stopServer();
  });
}
