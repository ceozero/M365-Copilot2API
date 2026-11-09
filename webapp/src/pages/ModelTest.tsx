import { useCallback, useEffect, useState } from "react";
import { Play } from "lucide-react";
import { api } from "../api";
import { t } from "../i18n";

type Model = { id: string; display_name?: string };
type Result = {
  status: "idle" | "testing" | "ok" | "fail";
  latencyMs?: number;
  reply?: string;
  error?: string;
};

type Push = (message: string, kind?: "success" | "error" | "info") => void;

export function ModelTestPage({ push }: { push: Push }) {
  const [models, setModels] = useState<Model[]>([]);
  const [results, setResults] = useState<Record<string, Result>>({});
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api("/api/admin/models");
      const nextModels: Model[] = data.data ?? [];
      setModels(nextModels);
      setResults((previous) => Object.fromEntries(
        nextModels.flatMap((model) => previous[model.id] ? [[model.id, previous[model.id]]] : [])
      ));
    } catch (error: any) {
      push(String(error?.message ?? error), "error");
    } finally {
      setLoading(false);
    }
  }, [push]);

  useEffect(() => { void load(); }, [load]);

  const runAll = useCallback(async () => {
    if (models.length === 0) return;
    setTesting(true);
    setResults((previous) => {
      const next = { ...previous };
      for (const model of models) next[model.id] = { status: "idle" };
      return next;
    });

    // 串行探测，避免“全部测试”对单个账号造成并发请求突发。
    for (const model of models) {
      const { id } = model;
      setResults((previous) => ({ ...previous, [id]: { status: "testing" } }));
      const started = performance.now();
      try {
        const data = await api("/api/admin/models/test", {
          method: "POST",
          body: JSON.stringify({ model: id }),
        });
        setResults((previous) => ({
          ...previous,
          [id]: {
            status: "ok",
            latencyMs: data.latency_ms ?? Math.round(performance.now() - started),
            reply: data.reply ?? "",
          },
        }));
      } catch (error: any) {
        setResults((previous) => ({
          ...previous,
          [id]: { status: "fail", error: String(error?.message ?? error) },
        }));
      }
    }
    setTesting(false);
  }, [models]);

  const statusFor = (result: Result) => {
    if (result.status === "ok") return { className: "online", label: "Healthy" };
    if (result.status === "fail") return { className: "offline", label: "Failed" };
    if (result.status === "testing") return { className: "cooldown", label: "Testing…" };
    return { className: "offline", label: "Not tested" };
  };

  return (
    <div>
      <div className="page-head">
        <div>
          <h2 className="page-title">{t("Model Test")}</h2>
          <p className="page-sub">{t("Send a minimal request to verify a model")}</p>
        </div>
        <button className="btn primary" type="button" onClick={() => void runAll()} disabled={testing || loading || models.length === 0}>
          <Play size={14} aria-hidden="true" /> {testing ? t("Testing…") : t("Test all")}
        </button>
      </div>

      <div className="card">
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>{t("Model")}</th>
                <th>{t("Status")}</th>
                <th>{t("Latency")}</th>
                <th>{t("Reply")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={4} className="empty">{t("Loading")}</td></tr>
              ) : models.length === 0 ? (
                <tr><td colSpan={4} className="empty">{t("No data")}</td></tr>
              ) : models.map((model) => {
                const result = results[model.id] ?? { status: "idle" as const };
                const status = statusFor(result);
                const reply = result.status === "fail" ? result.error : result.reply;
                return (
                  <tr key={model.id}>
                    <td><b>{model.id}</b></td>
                    <td><span className={`status ${status.className}`}><span className="dot" />{t(status.label)}</span></td>
                    <td style={{ color: "var(--muted)", fontVariantNumeric: "tabular-nums" }}>{result.latencyMs == null ? "-" : `${result.latencyMs} ms`}</td>
                    <td style={{ maxWidth: 420, fontSize: 12, color: result.status === "fail" ? "var(--red)" : "var(--muted)" }} title={reply}>
                      {reply ? (reply.length > 160 ? `${reply.slice(0, 160)}…` : reply) : "-"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
