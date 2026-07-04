interface TimelineEntry {
  level: number;
  set_at: string;
  note: string;
  teacher_name?: string;
  letter?: string;
  label?: string;
  description?: string;
}

interface CriterionEval {
  criterion_id: number;
  code: string;
  name: string;
  category: string;
  subcategory: string;
  current: TimelineEntry | null;
  history: TimelineEntry[];
}

const LEVEL_LETTERS = ["", "J", "Č", "T", "Ú"];
const LEVEL_LABELS = ["", "ještě neosvojeno", "částečně osvojeno", "téměř osvojeno", "úplně osvojeno"];

function levelColor(level: number): string {
  switch (level) {
    case 4: return "bg-green-500";
    case 3: return "bg-blue-500";
    case 2: return "bg-yellow-500";
    case 1: return "bg-red-500";
    default: return "bg-gray-400";
  }
}

function levelTextColor(level: number): string {
  switch (level) {
    case 4: return "text-green-700";
    case 3: return "text-blue-700";
    case 2: return "text-yellow-700";
    case 1: return "text-red-700";
    default: return "text-gray-500";
  }
}

function levelBorderColor(level: number): string {
  switch (level) {
    case 4: return "border-green-300 bg-green-50";
    case 3: return "border-blue-300 bg-blue-50";
    case 2: return "border-yellow-300 bg-yellow-50";
    case 1: return "border-red-300 bg-red-50";
    default: return "border-gray-300 bg-gray-50";
  }
}

function formatDate(s: string): string {
  return s.slice(0, 10).split("-").reverse().join(".");
}

export default function ProgressTimeline({
  evaluations,
  showTeacher = false,
}: {
  evaluations: CriterionEval[];
  showTeacher?: boolean;
}) {
  if (evaluations.length === 0) {
    return <p className="text-sm text-muted-foreground">Zatím nebylo zadáno žádné hodnocení.</p>;
  }

  const groups = groupByCategory(evaluations);
  const evaldOnly = evaluations.filter((e) => e.current !== null);

  return (
    <div className="space-y-6">
      {/* Summary stats */}
      <div className="flex gap-4 text-sm">
        <span className="text-muted-foreground">
          Hodnoceno: <b className="text-foreground">{evaldOnly.length}</b> / {evaluations.length} kritérií
        </span>
        {evaldOnly.length > 0 && (
          <span className="text-muted-foreground">
            Průměrná úroveň: <b className="text-foreground">{averageLevel(evaldOnly)}</b>
          </span>
        )}
      </div>

      {/* Level distribution bar */}
      {evaldOnly.length > 0 && (
        <LevelDistributionBar evaluations={evaldOnly} />
      )}

      {/* Per-criterion timeline */}
      {groups.map(({ category, subcategory, items }) => (
        <div key={category + subcategory}>
          <h3 className="text-sm font-semibold text-muted-foreground mb-2">
            {category}{subcategory && ` · ${subcategory}`}
          </h3>
          <div className="space-y-2">
            {items.map((ev) => {
              const all = collectTimeline(ev);
              return (
                <div
                  key={ev.criterion_id}
                  className={`rounded-lg border p-3 ${all.length > 0 ? levelBorderColor(all[all.length - 1].level) : "border-border bg-white"}`}
                >
                  <div className="flex items-start justify-between gap-2 mb-2">
                    <span className="text-xs">
                      <b>{ev.code}</b> {ev.name}
                    </span>
                    {all.length > 0 && (
                      <span className={`text-xs font-bold ${levelTextColor(all[all.length - 1].level)}`}>
                        {LEVEL_LETTERS[all[all.length - 1].level]}
                      </span>
                    )}
                  </div>

                  {all.length > 0 ? (
                    <div className="flex items-center gap-1 overflow-x-auto pb-1">
                      {all.map((entry, i) => (
                        <div key={i} className="flex items-center gap-1 shrink-0">
                          {i > 0 && <span className="text-muted-foreground text-xs">→</span>}
                          <div className="flex flex-col items-center min-w-[60px]">
                            <div className={`w-7 h-7 rounded-full ${levelColor(entry.level)} flex items-center justify-center text-white text-xs font-bold`}>
                              {LEVEL_LETTERS[entry.level]}
                            </div>
                            <span className="text-[10px] text-muted-foreground mt-0.5">
                              {formatDate(entry.set_at)}
                            </span>
                            {showTeacher && entry.teacher_name && (
                              <span className="text-[10px] text-muted-foreground italic">
                                {entry.teacher_name}
                              </span>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <span className="text-xs text-muted-foreground italic">Zatím nehodnoceno</span>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}

function LevelDistributionBar({ evaluations }: { evaluations: CriterionEval[] }) {
  const counts = [0, 0, 0, 0, 0];
  for (const ev of evaluations) {
    if (ev.current) {
      counts[ev.current.level]++;
    }
  }
  const total = evaluations.filter((e) => e.current).length;
  if (total === 0) return null;

  return (
    <div>
      <div className="flex h-6 rounded-md overflow-hidden border border-border">
        {[4, 3, 2, 1].map((level) => {
          const n = counts[level];
          if (n === 0) return null;
          const pct = (n / total) * 100;
          return (
            <div
              key={level}
              className={`${levelColor(level)} flex items-center justify-center text-white text-xs font-bold`}
              style={{ width: `${pct}%` }}
              title={`${LEVEL_LETTERS[level]} — ${LEVEL_LABELS[level]}: ${n}`}
            >
              {pct > 8 ? `${LEVEL_LETTERS[level]} ${n}` : ""}
            </div>
          );
        })}
      </div>
      <div className="flex gap-3 mt-1 text-xs text-muted-foreground">
        {[4, 3, 2, 1].map((level) => (
          <span key={level} className="flex items-center gap-1">
            <span className={`inline-block w-3 h-3 rounded-sm ${levelColor(level)}`} />
            {LEVEL_LETTERS[level]} ({counts[level]})
          </span>
        ))}
      </div>
    </div>
  );
}

function collectTimeline(ev: CriterionEval): TimelineEntry[] {
  const all = [...(ev.history || []), ...(ev.current ? [ev.current] : [])];
  return all.sort((a, b) => a.set_at.localeCompare(b.set_at));
}

function averageLevel(evaluations: CriterionEval[]): string {
  const evald = evaluations.filter((e) => e.current);
  if (evald.length === 0) return "—";
  const sum = evald.reduce((s, e) => s + (e.current?.level || 0), 0);
  return (sum / evald.length).toFixed(1);
}

function groupByCategory(evals: CriterionEval[]) {
  const groups: { category: string; subcategory: string; items: CriterionEval[] }[] = [];
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
