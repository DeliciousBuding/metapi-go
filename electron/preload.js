// Preload: exposes a tiny, safe bridge from the Electron main process to the
// rendered admin UI. The web UI runs in the browser sandbox; this only adds an
// informational surface — it does NOT grant any privileged Node access.
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('metapiDesktop', {
  // Desktop shell version (from package.json). Useful for the UI to show
  // "running in desktop v0.12.0" badges.
  appVersion: process.env.npm_package_version || 'dev',
  // Host platform (win32 / darwin / linux) for platform-aware UI tweaks.
  platform: process.platform,
  // Forwarded when the main process shows a native OS notification, so the SPA
  // can mirror/acknowledge it. Payload: { id, title, body }.
  onNativeNotification: (callback) =>
    ipcRenderer.on('metapi:native-notification', (_event, payload) => callback(payload)),
  // Acknowledge a desktop notification back to the main process (dedupe ack).
  ackNotification: (id) => ipcRenderer.send('metapi:ack-notification', id),
});
