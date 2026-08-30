import { useQuery } from "@tanstack/react-query";
import { fetchHealth } from "../api/health";

/**
 * APIとの疎通確認用の画面。Sprint 0 の動作確認に使う。
 * 業務画面が揃ったあとも、環境切り分け（フロントが悪いのかAPIが悪いのか）に役立つので残す。
 */
export function StatusPage() {
  const { data, error, isPending, refetch, isFetching } = useQuery({
    queryKey: ["health"],
    queryFn: ({ signal }) => fetchHealth(signal),
    refetchInterval: 10_000,
  });

  return (
    <section>
      <h2>稼働状況</h2>

      {isPending && <p>確認中…</p>}

      {error && (
        <p className="error" role="alert">
          APIに接続できません: {error.message}
          <br />
          <small>
            <code>docker compose -f deploy/docker-compose.yml up</code> が起動しているか確認してください。
          </small>
        </p>
      )}

      {data && (
        <dl className="status">
          <dt>API</dt>
          <dd>{data.status === "ok" ? "正常" : data.status}</dd>
          <dt>バージョン</dt>
          <dd>
            <code>{data.version}</code>
          </dd>
        </dl>
      )}

      <button type="button" onClick={() => void refetch()} disabled={isFetching}>
        {isFetching ? "確認中…" : "再確認"}
      </button>
    </section>
  );
}
