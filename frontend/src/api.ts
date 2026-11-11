import { Call, Events, System } from "@wailsio/runtime";
import { isPackagedDesktop } from "./desktop";
export type CredentialUpdate = {
  mode: "keep" | "none" | "plain" | "encrypted";
  value: string;
};
export type CredentialInfo = { mode: "plain" | "encrypted"; saved: boolean };
export type Mapping = {
  type: "local" | "remote" | "dynamic";
  bind: string;
  port: number;
  target: string;
  targetPort: number;
};
export type NetworkOptions = {
  connectTimeout: number;
  serverAliveInterval: number;
  serverAliveCountMax: number;
};
export const defaultNetwork: Readonly<NetworkOptions> = {
  connectTimeout: 15,
  serverAliveInterval: 15,
  serverAliveCountMax: 3,
};
export type Tunnel = {
  network?: NetworkOptions | null;
  id: string;
  name: string;
  host: string;
  port: number;
  user: string;
  auth: "agent" | "key" | "password";
  identity: string;
  jump: string;
  autoConnect: boolean;
  reconnect: boolean;
  mappings: Mapping[];
};
export type Settings = {
  theme: "light" | "dark";
  language: "system" | "en" | "zh-CN";
  sshPath: string;
  autoUpdate: boolean;
};
export type Config = { version: number; settings: Settings; tunnels: Tunnel[] };
export type Status = {
  isTest: boolean;
  command: string;
  id: string;
  state: string;
  code: string;
  attempt: number;
  since: string;
  logs: { time: string; code: string }[];
};
export type Challenge = {
  id: string;
  tunnelId: string;
  name: string;
  kind: string;
  prompt: string;
};
export type Snapshot = {
  configPath: string;
  credentials: Record<string, CredentialInfo>;
  tests: Status[];
  config: Config;
  statuses: Status[];
  challenges: Challenge[];
  sshAvailable: boolean;
  sshPath: string;
};
export const desktop = isPackagedDesktop(window.location) || System.IsDesktop();
export function call<T = void>(method: string, ...args: unknown[]): Promise<T> {
  if (!desktop) return Promise.reject(new Error("DESKTOP_REQUIRED"));
  return Call.ByName(`tunnelix/internal/service.Service.${method}`, ...args);
}
export function desktopCall<T = void>(method: string): Promise<T> {
  if (!desktop) return Promise.reject(new Error("DESKTOP_REQUIRED"));
  return Call.ByName(`main.Desktop.${method}`);
}
export function subscribe(fn: () => void) {
  return Events.On("tunnelix:changed", fn);
}
export const emptySnapshot: Snapshot = {
  configPath: "",
  credentials: {},
  tests: [],
  config: {
    version: 1,
    settings: {
      theme: "light",
      language: "system",
      sshPath: "",
      autoUpdate: true,
    },
    tunnels: [],
  },
  statuses: [],
  challenges: [],
  sshAvailable: false,
  sshPath: "",
};

export type UpdateStatus = {
  current: string;
  latest: string;
  notes: string;
  state: string;
  code: string;
  progress: number;
  checkedAt: string;
  installEnabled: boolean;
};
export const emptyUpdate: UpdateStatus = {
  current: "",
  latest: "",
  notes: "",
  state: "idle",
  code: "",
  progress: 0,
  checkedAt: "",
  installEnabled: false,
};
export { Events };
