const { contextBridge, ipcRenderer } = require('electron');

const subscribe = (channel, callback) => {
  const listener = (_event, payload) => callback(payload);
  ipcRenderer.on(channel, listener);
  return () => ipcRenderer.removeListener(channel, listener);
};

contextBridge.exposeInMainWorld('megafloyd', {
  info: () => ipcRenderer.invoke('megafloyd:info'),
  getState: () => ipcRenderer.invoke('megafloyd:state'),
  onState: (callback) => subscribe('megafloyd:state', callback),
  restart: () => ipcRenderer.invoke('megafloyd:restart'),
  openApp: () => ipcRenderer.invoke('megafloyd:open-app'),
  openLogs: () => ipcRenderer.invoke('megafloyd:open-logs'),
  openExternal: (url) => ipcRenderer.invoke('megafloyd:open-external', String(url)),
  copy: (text) => ipcRenderer.invoke('megafloyd:copy', String(text)),
  chooseWorkspace: () => ipcRenderer.invoke('megafloyd:choose-workspace'),
  handoff: (target) => ipcRenderer.invoke('megafloyd:handoff', String(target)),
  setBadge: (value) => ipcRenderer.invoke('megafloyd:set-badge', value),
  notify: (payload) => ipcRenderer.invoke('megafloyd:notify', payload),
});
