import { useState } from "react";
import { useTranslation } from "react-i18next";
import { FileJson } from "lucide-react";
import { desktopCall } from "../api";
import { Button } from "./ui/button";

export const configLocationEn = {
  title: "Configuration file",
  hint: "Changes are saved here. A new location is remembered for the next launch.",
  relocate: "Move to a new location",
  switch: "Use an existing file",
  behavior:
    "Moving keeps the original file and saved passwords. Switching loads the selected file without starting connections.",
  active: "Disconnect all tunnels and finish connection tests first.",
  unsaved: "Save or discard your settings changes before changing the file.",
};

export const configLocationZh: typeof configLocationEn = {
  title: "配置文件",
  hint: "修改会保存到此文件，更改位置后下次启动继续使用。",
  relocate: "迁移到新位置",
  switch: "使用已有文件",
  behavior: "迁移保留原文件与已保存密码；切换加载所选文件，不会立即自动连接。",
  active: "请先断开所有隧道并结束连接测试。",
  unsaved: "请先保存或取消本次设置修改，再更改配置文件。",
};

export function ConfigLocation({
  path,
  active,
  dirty,
  disabled,
  onBusy,
  onChanged,
}: {
  path: string;
  active: boolean;
  dirty: boolean;
  disabled: boolean;
  onBusy: (busy: boolean) => void;
  onChanged: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const blocked = disabled || pending || active || dirty || !path;
  async function change(method: "RelocateConfig" | "SwitchConfig") {
    if (blocked) return;
    setPending(true);
    setError("");
    onBusy(true);
    try {
      if (await desktopCall<boolean>(method)) await onChanged();
    } catch (e) {
      const code = String(e).match(/[A-Z][A-Z_]{3,}/)?.[0] || "";
      setError(t(`errors.${code}`, { defaultValue: t("unknownError") }));
    } finally {
      setPending(false);
      onBusy(false);
    }
  }
  return (
    <section className="settings-section settings-config">
      <div className="settings-label">
        <h3>
          <FileJson size={15} />
          {t("configLocation.title")}
        </h3>
        <p>{t("configLocation.hint")}</p>
      </div>
      <div className="config-location-controls">
        <input
          readOnly
          aria-label={t("configLocation.title")}
          value={path}
          title={path}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={blocked}
          onClick={() => void change("RelocateConfig")}
        >
          {t("configLocation.relocate")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={blocked}
          onClick={() => void change("SwitchConfig")}
        >
          {t("configLocation.switch")}
        </Button>
      </div>
      <p className="hint">
        {t(
          active
            ? "configLocation.active"
            : dirty
              ? "configLocation.unsaved"
              : "configLocation.behavior",
        )}
      </p>
      {error && (
        <p role="alert" className="test-failed">
          {error}
        </p>
      )}
    </section>
  );
}
