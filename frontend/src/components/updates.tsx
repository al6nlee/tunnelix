import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowDownToLine, RefreshCw } from "lucide-react";
import { desktopCall, type UpdateStatus } from "../api";
import { Button } from "./ui/button";

export const updateEn = {
  title: "Version & updates",
  current: "Current version",
  latest: "Available version",
  automatic: "Automatically check and download updates",
  hint: "Checks on startup and every 6 hours. You choose when to restart. Save to apply.",
  check: "Check for updates",
  download: "Download update",
  restart: "Restart to update",
  checked: "Last checked",
  notes: "Release notes",
  active:
    "Disconnect all tunnels and finish connection tests before restarting.",
  unsupported:
    "This build can check for updates; automatic installation is unavailable.",
  states: {
    idle: "Check for a newer version of Tunnelix.",
    checking: "Checking for updates…",
    "up-to-date": "You are up to date.",
    available: "A new version is available.",
    downloading: "Downloading and verifying update…",
    ready: "Update ready. Restart to install.",
    error: "Unable to update. Try again later.",
    unpublished: "No published release is available yet.",
    unsupported: "This build cannot install updates.",
    restarting: "Restarting…",
  },
  errors: {
    UPDATE_BUSY: "An update operation is already running.",
    UPDATE_UNAVAILABLE_BUILD: "This build cannot install updates.",
    UPDATE_ACTIVE_CONNECTIONS:
      "Disconnect all tunnels and finish connection tests first.",
    UPDATE_NOT_READY: "Download an update before restarting.",
    UPDATE_RESTART_FAILED:
      "Could not restart. Check that the application location is writable and try again.",
    UPDATE_CHECK_FAILED:
      "Could not check for updates. Check your network and try again.",
    UPDATE_DOWNLOAD_FAILED:
      "Download failed. Check your network and try again.",
    UPDATE_VERIFY_FAILED:
      "Update verification failed. The update was not installed.",
    UPDATE_ASSET_MISSING:
      "This release has no signed update for your platform.",
    UPDATE_NO_RELEASE: "No published release is available yet.",
  },
};
export const updateZh = {
  title: "版本与更新",
  current: "当前版本",
  latest: "可用版本",
  automatic: "自动检查并下载更新",
  hint: "启动时及每 6 小时检查，由你确认重启。保存后生效。",
  check: "检查更新",
  download: "下载更新",
  restart: "重启并更新",
  checked: "上次检查",
  notes: "更新说明",
  active: "请先断开所有隧道并结束连接测试，再重启更新。",
  unsupported: "当前版本可检查更新，暂不支持自动安装。",
  states: {
    idle: "检查是否有新版 Tunnelix。",
    checking: "正在检查更新…",
    "up-to-date": "当前已是最新版本。",
    available: "发现新版本。",
    downloading: "正在下载并验证更新…",
    ready: "更新已就绪，重启后完成安装。",
    error: "更新失败，请稍后重试。",
    unpublished: "目前还没有已发布的版本。",
    unsupported: "当前版本暂不支持安装更新。",
    restarting: "正在重启…",
  },
  errors: {
    UPDATE_BUSY: "更新操作正在进行。",
    UPDATE_UNAVAILABLE_BUILD: "当前版本暂不支持安装更新。",
    UPDATE_ACTIVE_CONNECTIONS: "请先断开所有隧道并结束连接测试。",
    UPDATE_NOT_READY: "请先下载更新，再重启安装。",
    UPDATE_RESTART_FAILED: "重启失败，请检查程序目录是否可写后重试。",
    UPDATE_CHECK_FAILED: "无法检查更新，请检查网络后重试。",
    UPDATE_DOWNLOAD_FAILED: "下载失败，请检查网络后重试。",
    UPDATE_VERIFY_FAILED: "更新验证失败，未安装此次更新。",
    UPDATE_ASSET_MISSING: "此版本暂未提供适用于当前系统的签名更新包。",
    UPDATE_NO_RELEASE: "目前还没有已发布的版本。",
  },
};

export function UpdatePanel({
  status,
  active,
  autoUpdate,
  onAutoUpdate,
  disabled,
}: {
  status: UpdateStatus;
  active: boolean;
  autoUpdate: boolean;
  onAutoUpdate: (value: boolean) => void;
  disabled: boolean;
}) {
  const { t, i18n } = useTranslation();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const busy =
    pending || ["checking", "downloading", "restarting"].includes(status.state);
  const errorText = (code: string) =>
    t(`updates.errors.${code}`, { defaultValue: t("unknownError") });
  async function act(method: string) {
    setPending(true);
    setError("");
    try {
      await desktopCall(method);
    } catch (e) {
      setError(errorText(String(e).match(/UPDATE_[A-Z_]+/)?.[0] || ""));
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="settings-section update-panel">
      <div className="update-header">
        <div className="settings-label">
          <h3>
            <RefreshCw size={15} />
            {t("updates.title")}
          </h3>
          <p>
            {t("updates.current")}{" "}
            <span className="version-tag">
              {status.current ? `v${status.current}` : "—"}
            </span>
            {status.latest && ` · ${t("updates.latest")} v${status.latest}`}
          </p>
        </div>
        <div className="update-actions">
          {status.state !== "ready" && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => void act("CheckForUpdates")}
            >
              <RefreshCw
                size={14}
                className={status.state === "checking" ? "spin" : ""}
              />
              {t("updates.check")}
            </Button>
          )}
          {status.state === "available" && status.installEnabled && (
            <Button
              type="button"
              size="sm"
              disabled={busy}
              onClick={() => void act("DownloadUpdate")}
            >
              <ArrowDownToLine size={14} />
              {t("updates.download")}
            </Button>
          )}
          {status.state === "ready" && (
            <Button
              type="button"
              size="sm"
              disabled={busy || active}
              onClick={() => void act("RestartUpdate")}
            >
              {t("updates.restart")}
            </Button>
          )}
        </div>
      </div>
      {status.state !== "idle" && (
        <p role="status" aria-live="polite" className="update-status">
          {t(`updates.states.${status.state}`)}
          {status.state === "downloading" && ` ${status.progress}%`}
        </p>
      )}
      {status.state === "downloading" && (
        <progress
          aria-label={t("updates.download")}
          value={status.progress}
          max={100}
        />
      )}
      {status.checkedAt && (
        <p className="hint">
          {t("updates.checked")} ·{" "}
          {new Date(status.checkedAt).toLocaleString(i18n.language)}
        </p>
      )}
      {(error ||
        (status.code &&
          !["unpublished", "unsupported"].includes(status.state))) && (
        <p role="alert" className="test-failed">
          {error || errorText(status.code)}
        </p>
      )}
      {status.state === "ready" && active && (
        <p className="hint">{t("updates.active")}</p>
      )}
      <label className="settings-switch-row">
        <span>
          <strong>{t("updates.automatic")}</strong>
          <small>{t("updates.hint")}</small>
        </span>
        <input
          type="checkbox"
          role="switch"
          checked={autoUpdate}
          disabled={disabled}
          onChange={(e) => onAutoUpdate(e.target.checked)}
        />
        <span className="switch-track" aria-hidden="true" />
      </label>
      {!status.installEnabled && (
        <p className="hint update-limit">{t("updates.unsupported")}</p>
      )}
      {status.notes && (
        <details>
          <summary>{t("updates.notes")}</summary>
          <pre className="update-notes">{status.notes}</pre>
        </details>
      )}
    </section>
  );
}
