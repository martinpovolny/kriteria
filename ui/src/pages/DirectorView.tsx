import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import {
  apiGet, type DirectorStudent, type AuditEntry, type Evaluation,
} from "@/lib/api";
import ProgressTimeline from "@/components/ProgressTimeline";

type Tab = "students" | "progress" | "audit";

export default function DirectorView() {
  const [tab, setTab] = useState<Tab>("students");
  const [students, setStudents] = useState<DirectorStudent[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setLoading(true);
    if (tab === "students" || tab === "progress") {
      apiGet<DirectorStudent[]>("/api/director/students")
        .then((d) => setStudents(Array.isArray(d) ? d : []))
        .catch(console.error)
        .finally(() => setLoading(false));
    } else {
      apiGet<AuditEntry[]>("/api/audit?limit=100")
        .then((d) => setAudit(Array.isArray(d) ? d : []))
        .catch(console.error)
        .finally(() => setLoading(false));
    }
  }, [tab]);

  return (
    <div className="min-h-full bg-background text-foreground">
      <header className="border-b border-border px-6 py-3 flex items-center gap-4">
        <Link to="/" className="text-lg font-bold">Kriteria</Link>
        <span className="text-muted-foreground">/</span>
        <span className="text-sm font-medium">Ředitel</span>
        <div className="ml-auto flex items-center gap-4">
          <Link to="/prehled-pristupu" className="text-sm text-muted-foreground hover:text-foreground">
            Přístupové kódy →
          </Link>
          <Link to="/prehled" className="text-sm text-muted-foreground hover:text-foreground">
            Přehled kritérií →
          </Link>
          <a href="/api/auth/logout" className="text-sm text-muted-foreground hover:text-foreground">
            Odhlásit
          </a>
        </div>
      </header>

      {/* Tabs */}
      <div className="border-b border-border px-6 flex gap-1">
        <TabButton active={tab === "students"} onClick={() => setTab("students")}>
          Žáci ({students.length})
        </TabButton>
        <TabButton active={tab === "progress"} onClick={() => setTab("progress")}>
          Postup v čase
        </TabButton>
        <TabButton active={tab === "audit"} onClick={() => setTab("audit")}>
          Záznamy hodnocení ({audit.length})
        </TabButton>
      </div>

      <div className="p-6">
        {loading ? (
          <p className="text-muted-foreground text-sm">Načítám…</p>
        ) : tab === "students" ? (
          <StudentsTab students={students} />
        ) : tab === "progress" ? (
          <ProgressTab students={students} />
        ) : (
          <AuditTab entries={audit} />
        )}
      </div>
    </div>
  );
}

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
        active
          ? "border-primary text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}

function StudentsTab({ students }: { students: DirectorStudent[] }) {
  return (
    <div className="max-w-4xl">
      <h2 className="text-lg font-semibold mb-4">Seznam žáků</h2>
      <div className="space-y-2">
        {students.map((s) => (
          <div key={s.id} className="rounded-lg border border-border p-4">
            <div className="flex items-center justify-between mb-2">
              <span className="font-medium">{s.display_name}</span>
              <span className="text-xs text-muted-foreground">
                {s.enrollments.length} {s.enrollments.length === 1 ? "zápis" : "zápisů"}
              </span>
            </div>
            {s.enrollments.length > 0 && (
              <div className="flex flex-wrap gap-2">
                {s.enrollments.map((e, i) => (
                  <span
                    key={i}
                    className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-1 text-xs"
                  >
                    {e.subject_code} — {e.grade_level}. roč. · {e.school_year}
                  </span>
                ))}
              </div>
            )}
            {s.enrollments.length === 0 && (
              <p className="text-xs text-muted-foreground italic">Bez zápisů</p>
            )}
          </div>
        ))}
        {students.length === 0 && (
          <p className="text-sm text-muted-foreground">Žádní žáci</p>
        )}
      </div>
    </div>
  );
}

function AuditTab({ entries }: { entries: AuditEntry[] }) {
  const levelLetter = (level: number) => ["", "J", "Č", "T", "Ú"][level] || "?";
  const levelClass = (level: number) =>
    level === 4 ? "bg-green-100 text-green-900" :
    level === 3 ? "bg-blue-100 text-blue-900" :
    level === 2 ? "bg-yellow-100 text-yellow-900" :
    "bg-red-100 text-red-900";

  return (
    <div className="max-w-5xl">
      <h2 className="text-lg font-semibold mb-4">Záznamy hodnocení (kdo, kdy, co)</h2>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground">
              <th className="py-2 pr-4 font-medium">Kdy</th>
              <th className="py-2 pr-4 font-medium">Učitel</th>
              <th className="py-2 pr-4 font-medium">Žák</th>
              <th className="py-2 pr-4 font-medium">Kritérium</th>
              <th className="py-2 pr-4 font-medium">Předmět</th>
              <th className="py-2 pr-4 font-medium text-center">Úroveň</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.id} className="border-b border-border/50 hover:bg-muted/50">
                <td className="py-2 pr-4 text-muted-foreground whitespace-nowrap">
                  {e.set_at.slice(0, 16).replace("T", " ")}
                </td>
                <td className="py-2 pr-4">{e.teacher_name}</td>
                <td className="py-2 pr-4">{e.student_name}</td>
                <td className="py-2 pr-4">
                  <span className="font-mono text-xs">{e.criterion_code}</span>
                  {" "}
                  <span className="text-muted-foreground">{e.criterion_name}</span>
                </td>
                <td className="py-2 pr-4 whitespace-nowrap">
                  {e.subject_code} {e.grade_level}. roč.
                </td>
                <td className="py-2 pr-4 text-center">
                  <span className={`inline-block rounded px-2 py-0.5 text-xs font-bold ${levelClass(e.level)}`}>
                    {levelLetter(e.level)}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {entries.length === 0 && (
        <p className="text-sm text-muted-foreground">Žádné záznamy</p>
      )}
    </div>
  );
}

function ProgressTab({ students }: { students: DirectorStudent[] }) {
  const [selectedStudentId, setSelectedStudentId] = useState<number>(0);
  const [evaluations, setEvaluations] = useState<Evaluation[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!selectedStudentId) {
      setEvaluations([]);
      return;
    }
    setLoading(true);
    apiGet<Evaluation[]>(`/api/evaluations?student_id=${selectedStudentId}`)
      .then((d) => setEvaluations(Array.isArray(d) ? d : []))
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [selectedStudentId]);

  return (
    <div className="max-w-4xl">
      <h2 className="text-lg font-semibold mb-4">Postup žáka v čase</h2>

      {/* Student selector */}
      <div className="mb-6">
        <select
          value={selectedStudentId}
          onChange={(e) => setSelectedStudentId(Number(e.target.value))}
          className="w-full max-w-md rounded-md border border-border px-3 py-2 text-sm"
        >
          <option value={0}>— vyberte žáka —</option>
          {students.map((s) => (
            <option key={s.id} value={s.id}>{s.display_name}</option>
          ))}
        </select>
      </div>

      {selectedStudentId === 0 ? (
        <p className="text-sm text-muted-foreground">Vyberte žáka vlevo.</p>
      ) : loading ? (
        <p className="text-sm text-muted-foreground">Načítám…</p>
      ) : (
        <ProgressTimeline evaluations={evaluations} showTeacher={true} />
      )}
    </div>
  );
}
