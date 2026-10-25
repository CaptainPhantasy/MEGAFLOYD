import path from 'node:path';

/**
 * Application menu. Actions are injected so this module has no coupling to
 * window or engine internals.
 */
export function buildMenu(Menu, actions) {
  const isMac = process.platform === 'darwin';

  const template = [
    ...(isMac
      ? [
          {
            label: 'MegaFloyd',
            submenu: [
              { role: 'about', label: 'About MegaFloyd' },
              { label: 'Check for Updates…', click: actions.checkUpdates },
              { type: 'separator' },
              { label: 'Engine Status', accelerator: 'CmdOrCtrl+,', click: actions.showStatus },
              { label: 'Restart Engine', accelerator: 'Shift+CmdOrCtrl+R', click: actions.restartEngine },
              { type: 'separator' },
              { role: 'services' },
              { type: 'separator' },
              { role: 'hide', label: 'Hide MegaFloyd' },
              { role: 'hideOthers' },
              { role: 'unhide' },
              { type: 'separator' },
              { role: 'quit', label: 'Quit MegaFloyd' },
            ],
          },
        ]
      : []),
    {
      label: 'File',
      submenu: [
        { label: 'New Window', accelerator: 'CmdOrCtrl+N', click: actions.newWindow },
        { label: 'Open Workspace…', accelerator: 'Shift+CmdOrCtrl+O', click: actions.chooseWorkspace },
        { type: 'separator' },
        { label: 'Copy Local URL', accelerator: 'Shift+CmdOrCtrl+C', click: actions.copyUrl },
        { label: 'Open in Browser', click: actions.openInBrowser },
        { label: 'Open in Terminal', click: actions.openInTerminal },
        { label: 'Drive Access Settings…', click: actions.driveAccessSettings },
        { label: 'Show App for Drive Access', click: actions.revealApp },
        { type: 'separator' },
        isMac ? { role: 'close' } : { role: 'quit' },
      ],
    },
    {
      label: 'Engine',
      submenu: [
        { label: 'Restart Engine', accelerator: 'Shift+CmdOrCtrl+E', click: actions.restartEngine },
        { label: 'Reload Interface', accelerator: 'CmdOrCtrl+R', click: actions.reload },
        { type: 'separator' },
        { label: 'Open Engine Logs', click: actions.openLogs },
        { label: 'Reveal MegaFloyd Home', click: actions.revealHome },
        { type: 'separator' },
        { label: 'Toggle Developer Tools', accelerator: 'Alt+CmdOrCtrl+I', click: actions.toggleDevTools },
      ],
    },
    {
      label: 'Edit',
      submenu: [
        { role: 'undo' },
        { role: 'redo' },
        { type: 'separator' },
        { role: 'cut' },
        { role: 'copy' },
        { role: 'paste' },
        { role: 'selectAll' },
      ],
    },
    {
      label: 'View',
      submenu: [
        { role: 'resetZoom' },
        { role: 'zoomIn' },
        { role: 'zoomOut' },
        { type: 'separator' },
        { role: 'togglefullscreen' },
      ],
    },
    {
      label: 'Window',
      submenu: [
        { role: 'minimize' },
        { role: 'zoom' },
        ...(isMac ? [{ type: 'separator' }, { role: 'front' }] : [{ role: 'close' }]),
      ],
    },
    {
      role: 'help',
      submenu: [
        { label: 'MegaFloyd Documentation', click: actions.openDocs },
        { label: 'Project on GitHub', click: actions.openProject },
      ],
    },
  ];

  return Menu.buildFromTemplate(template);
}

export function trayIconPath(resourcesPath) {
  return path.join(resourcesPath, 'icon.icns');
}
