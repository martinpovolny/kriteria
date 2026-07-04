import { useState, useEffect } from "react";
import { useParams } from "react-router-dom";
import { apiPost, apiGet } from "@/lib/api";
import ProgressTimeline from "@/components/ProgressTimeline";

interface VerifyResp {
  token: string;
  student_id: number;
  subject_id?: number;
  grade_id?: number;
  school_year_id?: number;
}

interface ParentEvaluation {
  criterion_id: number;
  code: string;
  name: string;
  category: string;
  subcategory: string;
  current: {
    level: number;
    letter: string;
    label: string;
    description: string;
    set_at: string;
    note: string;
  } | null;
  history: {
    level: number;
    letter: string;
    label: string;
    description: string;
    set_at: string;
    note: string;
  }[];
}

type View = "current" | "progress";

export default function ParentView() {
  const { slug } = useParams<{ slug: string }>();
  const [password, setPassword] = useState("");
  const [token, setToken] = useState("");
  const [verified, setVerified] = useState(false);
  const [error, setError] = useState("");
  const [evaluations, setEvaluations] = useState<ParentEvaluation[]>([]);
  const [loading, setLoading] = useState(false);
  const [view, setView] = useState<View>("current");

  const handleVerify = async () => {
    setError("");
    setLoading(true);
    try {
      const resp = await apiPost<VerifyResp>("/api/parent/verify", {
        slug: slug || "",
        password,
      });
      setToken(resp.token);
      setVerified(true);
      localStorage.setItem("parent_token", resp.token);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Chyba");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (verified && token) {
      apiGet<{ evaluations: ParentEvaluation[] }>(
        `/api/parent/evaluations?token=${token}`
      )
        .then((data) => setEvaluations(data.evaluations || []))
        .catch(console.error);
    }
  }, [verified, token]);

  // Auto-restore session
  useEffect(() => {
    const saved = localStorage.getItem("parent_token");
    if (saved) {
      setToken(saved);
      setVerified(true);
    }
  }, []);

  if (!verified) {
    return (
      <div className="min-h-full flex items-center justify-center bg-background px-4">
        <div className="rounded-lg border border-border p-8 max-w-md w-full">
          <h1 className="text-xl font-bold mb-2">Hodnocení žáka</h1>
          <p className="text-muted-foreground text-sm mb-6">
            Zadejte heslo, které jste obdrželi od školy.
          </p>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleVerify()}
            placeholder="např. zelenatrave"
            className="w-full rounded-md border border-border px-3 py-2 text-sm mb-3"
            autoFocus
          />
          {error && <p className="text-sm text-red-600 mb-3">{error}</p>}
          <button
            onClick={handleVerify}
            disabled={loading || !password}
            className="w-full rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
          >
            {loading ? "Ověřuji…" : "Zobrazit hodnocení"}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-full bg-background text-foreground">
      <header className="border-b border-border px-6 py-3">
        <h1 className="text-lg font-bold">Hodnocení žáka</h1>
        <p className="text-xs text-muted-foreground">Anonymní přístup</p>
      </header>

      <div className="max-w-3xl mx-auto p-6">
        {evaluations.length === 0 ? (
          <p className="text-sm text-muted-foreground">Zatím nebylo zadáno žádné hodnocení.</p>
        ) : (
          <>
            {/* View toggle */}
            <div className="flex gap-1 mb-6 border-b border-border">
              <button
                onClick={() => setView("current")}
                className={`px-4 py-2 text-sm font-medium border-b-2 ${
                  view === "current" ? "border-primary text-foreground" : "border-transparent text-muted-foreground"
                }`}
              >
                Aktuální hodnocení
              </button>
              <button
                onClick={() => setView("progress")}
                className={`px-4 py-2 text-sm font-medium border-b-2 ${
                  view === "progress" ? "border-primary text-foreground" : "border-transparent text-muted-foreground"
                }`}
              >
                Postup v čase
              </button>
            </div>

            {view === "progress" ? (
              <ProgressTimeline evaluations={evaluations} showTeacher={false} />
            ) : (
              groupByCategory(evaluations).map(({ category, subcategory, items }) => (
            <div key={category + subcategory} className="mb-6">
              <h3 className="text-sm font-semibold text-muted-foreground mb-2">
                {category}{subcategory && ` · ${subcategory}`}
              </h3>
              {items.map((ev) => (
                <div key={ev.criterion_id} className="rounded-lg border border-border p-4 mb-3">
                  <div className="flex items-start justify-between gap-4 mb-2">
                    <span className="text-sm">
                      <b>{ev.code}</b> {ev.name}
                    </span>
                    {ev.current && (
                      <span className={`text-xs font-bold px-2 py-1 rounded shrink-0 ${levelClass(ev.current.level)}`}>
                        {ev.current.letter} — {ev.current.label}
                      </span>
                    )}
                  </div>
                  {ev.current && (
                    <p className="text-xs text-muted-foreground italic">
                      {ev.current.description}
                    </p>
                  )}
                  {ev.history.length > 0 && (
                    <details className="mt-2">
                      <summary className="text-xs text-muted-foreground cursor-pointer">
                        Historie ({ev.history.length})
                      </summary>
                      <div className="mt-2 space-y-1">
                        {ev.history.map((h, i) => (
                          <div key={i} className="text-xs text-muted-foreground">
                            <b>{h.letter}</b> — {h.set_at.slice(0, 10)}
                            {h.note && ` · ${h.note}`}
                          </div>
                        ))}
                      </div>
                    </details>
                  )}
                </div>
              ))}
            </div>
              ))
            )}
          </>
        )}
      </div>
    </div>
  );
}

function levelClass(level: number): string {
  switch (level) {
    case 4: return "bg-green-100 text-green-800";
    case 3: return "bg-blue-100 text-blue-800";
    case 2: return "bg-yellow-100 text-yellow-800";
    case 1: return "bg-red-100 text-red-800";
    default: return "bg-gray-100 text-gray-800";
  }
}

function groupByCategory(evals: ParentEvaluation[]) {
  const groups: { category: string; subcategory: string; items: ParentEvaluation[] }[] = [];
  const seen = new Set<string>();
  for (const e of evals) {
    const key = e.category + "|" + e.subcategory;
    if (!seen.has(key)) {
      seen.add(key);
      groups.push({ category: e.category, subcategory: e.subcategory, items: [] });
    }
    const g = groups.find((g) => g.category === e.category && g.subcategory === e.subcategory);
    g!.items.push(e);
  }
  return groups;
}
