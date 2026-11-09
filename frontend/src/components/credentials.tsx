import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Eye, EyeOff } from "lucide-react";
import { Button } from "./ui/button";
import type { CredentialInfo, CredentialUpdate } from "../api";

export function PasswordInput({
  value,
  onChange,
  placeholder,
  required = false,
  autoFocus = false,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  required?: boolean;
  autoFocus?: boolean;
}) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  return (
    <div className="input-action">
      <input
        aria-label={t("password")}
        type={visible ? "text" : "password"}
        autoComplete="off"
        spellCheck={false}
        autoFocus={autoFocus}
        required={required}
        maxLength={8192}
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
      />
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label={t(visible ? "hidePassword" : "showPassword")}
        aria-pressed={visible}
        onClick={() => setVisible(!visible)}
      >
        {visible ? <EyeOff size={16} /> : <Eye size={16} />}
      </Button>
    </div>
  );
}
export function CredentialFields({
  info,
  update,
  onChange,
  identityChanged,
}: {
  info?: CredentialInfo;
  update: CredentialUpdate;
  onChange: (update: CredentialUpdate) => void;
  identityChanged: boolean;
}) {
  const { t } = useTranslation();
  const mode = update.mode === "keep" ? (info?.mode ?? "none") : update.mode;
  return (
    <section className="credential-fields">
      <label className="field">
        <span>{t("passwordStorage")}</span>
        <select
          value={mode}
          onChange={(e) =>
            onChange({
              mode: e.target.value as CredentialUpdate["mode"],
              value: update.value,
            })
          }
        >
          <option value="none">{t("storageNone")}</option>
          <option value="plain">{t("storagePlain")}</option>
          <option value="encrypted">{t("storageEncrypted")}</option>
        </select>
      </label>
      <label className="field">
        <span>
          {t("password")}
          {info?.saved && (
            <small className="saved-indicator"> · {t("passwordSaved")}</small>
          )}
        </span>
        <PasswordInput
          value={update.value}
          placeholder={t(
            info?.saved && mode !== "none"
              ? "keepPassword"
              : "passwordOptional",
          )}
          required={mode !== "none" && !info?.saved}
          onChange={(value) =>
            onChange({ mode: mode as CredentialUpdate["mode"], value })
          }
        />
      </label>
      <p className={`hint ${mode === "plain" ? "credential-warning" : ""}`}>
        {t(
          mode === "plain"
            ? "plainHint"
            : mode === "encrypted"
              ? "encryptedHint"
              : "unsavedPasswordHint",
        )}
      </p>
      {info?.saved && mode === "none" && (
        <p className="hint">{t("removePasswordHint")}</p>
      )}
      {identityChanged && <p className="hint">{t("identityCredentialHint")}</p>}
    </section>
  );
}
