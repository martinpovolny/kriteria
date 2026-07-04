import { useState, useEffect, useCallback } from "react";
import { Link } from "react-router-dom";
import {
  apiGet, apiPost, type Subject, type Student, type Criterion, type Evaluation, type SchoolYear,
} from "@/lib/api";
import ProgressTimeline from "@/components/ProgressTimeline";

type View = "current" | "progress";

export default function TeacherView() {
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [schoolYears, setSchoolYears] = useState<SchoolYear[]>([]);
  const [selectedSchoolYear, setSelectedSchoolYear] = useState<number>(0);
  const [selectedSubject, setSelectedSubject] = useState<string>("");
  const [selectedGrade, setSelectedGrade] = useState<number>(0);
  const [students, setStudents] = useState<Student[]>([]);
  const [criteria, setCriteria] = useState<Criterion[]>([]);
  const [selectedStudent, setSelectedStudent] = useState<Student | null>(null);
  const [evaluations, setEvaluations] = useState<Evaluation[]>([]);
  const [loading, setLoading] = useState(false);
  const [showAddStudent, setShowAddStudent] = useState(false);
  const [newStudentName, setNewStudentName] = useState("");
  const [view, setView] = useState<View>("current");

  useEffect(() => {
    apiGet<Subject[]>("/api/subjects").then(setSubjects).catch(console.error);
    // Load school years + default to current
    Promise.all([
      apiGet<SchoolYear[]>("/api/school-years"),
      apiGet<{ id: number; label: string }>("/api/school-years/current"),
    ]).then(([years, current]) => {
      setSchoolYears(years);
      setSelectedSchoolYear(current.id);
    }).catch(console.error);
  }, []);

  // Load students filtered by subject+grade enrollment + school year
  useEffect(() => {
    if (selectedSubject && selectedGrade && selectedSchoolYear) {
      const subj = subjects.find((s) => s.code === selectedSubject);
      const grade = subj?.grades.find((g) => g.level === selectedGrade);
      if (subj && grade) {
        apiGet<Student[]>(`/api/students?subject_id=${subj.id}&grade_id=${grade.id}&school_year_id=${selectedSchoolYear}`)
          .then((d) => setStudents(Array.isArray(d) ? d : []))
          .catch(() => setStudents([]));
      }
    } else {
      setStudents([]);
    }
  }, [selectedSubject, selectedGrade, selectedSchoolYear, subjects]);

  // Load criteria when subject+grade selected
  useEffect(() => {
    if (selectedSubject && selectedGrade) {
      apiGet<Criterion[]>(`/api/criteria/${selectedSubject}/${selectedGrade}`)
        .then((d) => setCriteria(Array.isArray(d) ? d : []))
        .catch(console.error);
    } else {
      setCriteria([]);
    }
  }, [selectedSubject, selectedGrade]);

  // Load evaluations when student selected
  const loadEvaluations = useCallback(async (studentId: number) => {
    if (!selectedSubject || !selectedGrade) return;
    setLoading(true);
    try {
      // Get subject ID from the selected subject
      const subj = subjects.find((s) => s.code === selectedSubject);
      if (!subj) return;
      const grade = subj.grades.find((g) => g.level === selectedGrade);
      if (!grade) return;
      // We need subject_id and grade_id for the API. Let's use the criteria endpoint to get them.
      // Actually the evaluations API takes student_id + optional subject_id + grade_id
      // But we only have subject code and grade level. Let's just filter by student for now.
      const evals = await apiGet<Evaluation[]>(
        `/api/evaluations?student_id=${studentId}`
      );
      setEvaluations(Array.isArray(evals) ? evals : []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [selectedSubject, selectedGrade, subjects]);

  useEffect(() => {
    if (selectedStudent) {
      loadEvaluations(selectedStudent.id);
    } else {
      setEvaluations([]);
    }
  }, [selectedStudent, loadEvaluations]);

  const handleSetLevel = async (criterionId: number, level: number) => {
    if (!selectedStudent) return;
    try {
      await apiPost("/api/evaluations", {
        student_id: selectedStudent.id,
        criterion_id: criterionId,
        level,
      });
      loadEvaluations(selectedStudent.id);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Chyba");
    }
  };

  const handleAddStudent = async () => {
    if (!newStudentName.trim()) return;
    try {
      await apiPost("/api/students", { display_name: newStudentName.trim() });
      setNewStudentName("");
      setShowAddStudent(false);
      apiGet<Student[]>("/api/students").then(setStudents);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Chyba");
    }
  };

  // All grades available across all subjects
  const allGrades = [...new Set(subjects.flatMap((s) => s.grades.map((g) => g.level)))].sort((a, b) => a - b);

  // Subjects that have the selected grade
  const subjectsForGrade = selectedGrade > 0
    ? subjects.filter((s) => s.grades.some((g) => g.level === selectedGrade))
    : [];

  return (
    <div className="min-h-full bg-background text-foreground">
      <header className="border-b border-border px-6 py-3 flex items-center gap-4">
        <Link to="/" className="text-lg font-bold">Kriteria</Link>
        <span className="text-muted-foreground">/</span>
        <span className="text-sm font-medium">Učitel (dev)</span>
        <div className="ml-auto flex items-center gap-4">
          <Link to="/prehled" className="text-sm text-muted-foreground hover:text-foreground">
            Přehled kritérií →
          </Link>
          <a href="/api/auth/logout" className="text-sm text-muted-foreground hover:text-foreground">
            Odhlásit
          </a>
        </div>
      </header>

      <div className="flex h-[calc(100vh-53px)]">
        {/* Left sidebar: filters + student list */}
        <div className="w-80 border-r border-border overflow-y-auto flex flex-col">
          {/* School year selector */}
          <div className="p-4 border-b border-border">
            <label className="block text-xs font-medium text-muted-foreground mb-1">Školní rok</label>
            <select
              value={selectedSchoolYear}
              onChange={(e) => { setSelectedSchoolYear(Number(e.target.value)); setSelectedStudent(null); }}
              className="w-full rounded-md border border-border px-3 py-2 text-sm"
            >
              {schoolYears.map((sy) => (
                <option key={sy.id} value={sy.id}>{sy.label}</option>
              ))}
            </select>
          </div>

          {/* Grade selector (first) */}
          <div className="p-4 border-b border-border">
            <label className="block text-xs font-medium text-muted-foreground mb-1">Ročník</label>
            <select
              value={selectedGrade}
              onChange={(e) => { setSelectedGrade(Number(e.target.value)); setSelectedSubject(""); setSelectedStudent(null); }}
              className="w-full rounded-md border border-border px-3 py-2 text-sm"
            >
              <option value={0}>— vyberte —</option>
              {allGrades.map((g) => (
                <option key={g} value={g}>{g}. ročník</option>
              ))}
            </select>
          </div>

          {/* Subject selector (second, filtered by grade) */}
          {selectedGrade > 0 && (
            <div className="p-4 border-b border-border">
              <label className="block text-xs font-medium text-muted-foreground mb-1">Předmět</label>
              <select
                value={selectedSubject}
                onChange={(e) => { setSelectedSubject(e.target.value); setSelectedStudent(null); }}
                className="w-full rounded-md border border-border px-3 py-2 text-sm"
              >
                <option value="">— vyberte —</option>
                {subjectsForGrade.map((s) => (
                  <option key={s.code} value={s.code}>{s.code} — {s.name}</option>
                ))}
              </select>
            </div>
          )}

          {/* Student list */}
          {selectedGrade > 0 && (
            <div className="flex-1 overflow-y-auto">
              <div className="px-4 pt-4 pb-2 flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">Žáci ({students.length})</span>
                <button
                  onClick={() => setShowAddStudent(!showAddStudent)}
                  className="text-xs text-foreground hover:underline"
                >
                  + přidat
                </button>
              </div>
              {showAddStudent && (
                <div className="px-4 pb-3 flex gap-2">
                  <input
                    type="text"
                    value={newStudentName}
                    onChange={(e) => setNewStudentName(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && handleAddStudent()}
                    placeholder="Jméno žáka"
                    className="flex-1 rounded-md border border-border px-2 py-1 text-sm"
                    autoFocus
                  />
                  <button onClick={handleAddStudent} className="rounded-md bg-primary px-3 py-1 text-xs text-primary-foreground">
                    OK
                  </button>
                </div>
              )}
              {students.map((s) => (
                <button
                  key={s.id}
                  onClick={() => setSelectedStudent(s)}
                  className={`w-full text-left px-4 py-2 text-sm hover:bg-muted transition-colors ${
                    selectedStudent?.id === s.id ? "bg-muted font-medium" : ""
                  }`}
                >
                  {s.display_name}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Main panel: criteria + evaluations */}
        <div className="flex-1 overflow-y-auto">
          {!selectedStudent ? (
            <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
              {!selectedSubject || !selectedGrade
                ? "Vyberte předmět a ročník"
                : "Vyberte žáka vlevo"}
            </div>
          ) : loading ? (
            <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
              Načítám…
            </div>
          ) : (
            <div className="p-6 max-w-4xl">
              <h2 className="text-lg font-semibold mb-1">{selectedStudent.display_name}</h2>
              <p className="text-sm text-muted-foreground mb-4">
                {selectedSubject} — {selectedGrade}. ročník · {schoolYears.find(sy => sy.id === selectedSchoolYear)?.label} · {criteria.length} kritérií
              </p>

              {/* View toggle */}
              <div className="flex gap-1 mb-6 border-b border-border">
                <button
                  onClick={() => setView("current")}
                  className={`px-4 py-2 text-sm font-medium border-b-2 ${
                    view === "current" ? "border-primary text-foreground" : "border-transparent text-muted-foreground"
                  }`}
                >
                  Hodnocení
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
                <ProgressTimeline evaluations={evaluations} showTeacher={true} />
              ) : (
                <>
              {/* Group criteria by category */}
              {groupByCategory(criteria).map(({ category, subcategory, items }) => (
                <div key={category + subcategory} className="mb-6">
                  <h3 className="text-sm font-semibold text-muted-foreground mb-2">
                    {category}{subcategory && ` · ${subcategory}`}
                  </h3>
                  {items.map((crit) => {
                    const eval_ = evaluations.find((e) => e.criterion_id === crit.id);
                    const currentLevel = eval_?.current?.level ?? 0;
                    return (
                      <div key={crit.id} className="rounded-lg border border-border p-4 mb-3">
                        <div className="flex items-start justify-between gap-4 mb-3">
                          <div>
                            <span className="font-medium text-sm">{crit.code}</span>{" "}
                            <span className="text-sm">{crit.name}</span>
                          </div>
                          {currentLevel > 0 && eval_?.current && (
                            <span className="text-xs text-muted-foreground shrink-0">
                              {eval_.current.set_at.slice(0, 10)} · {eval_.current.teacher_name}
                            </span>
                          )}
                        </div>

                        {/* Level buttons */}
                        <div className="grid grid-cols-4 gap-2">
                          {(crit.levels || []).map((lv) => (
                            <button
                              key={lv.level}
                              onClick={() => handleSetLevel(crit.id, lv.level)}
                              className={`rounded-md px-3 py-2 text-left text-xs transition-all border ${
                                currentLevel === lv.level
                                  ? levelActiveClass(lv.level)
                                  : "border-border bg-white hover:bg-muted"
                              }`}
                              title={lv.description}
                            >
                              <div className="font-bold mb-0.5">{lv.letter}</div>
                              <div className="text-muted-foreground">{lv.label}</div>
                            </button>
                          ))}
                        </div>

                        {/* Show description of current level */}
                        {currentLevel > 0 && (() => {
                          const lv = crit.levels.find((l) => l.level === currentLevel);
                          return lv ? (
                            <p className="mt-2 text-xs text-muted-foreground italic">{lv.description}</p>
                          ) : null;
                        })()}
                      </div>
                    );
                  })}
                </div>
              ))}

              {criteria.length === 0 && (
                <p className="text-sm text-muted-foreground">Pro tento předmět a ročník nejsou k dispozici kritéria.</p>
              )}
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function levelActiveClass(level: number): string {
  switch (level) {
    case 4: return "bg-green-100 border-green-300 text-green-900";
    case 3: return "bg-blue-100 border-blue-300 text-blue-900";
    case 2: return "bg-yellow-100 border-yellow-300 text-yellow-900";
    case 1: return "bg-red-100 border-red-300 text-red-900";
    default: return "border-border";
  }
}

function groupByCategory(criteria: Criterion[]) {
  const groups: { category: string; subcategory: string; items: Criterion[] }[] = [];
  const seen = new Set<string>();
  for (const c of criteria) {
    const key = c.category + "|" + c.subcategory;
    if (!seen.has(key)) {
      seen.add(key);
      groups.push({ category: c.category, subcategory: c.subcategory, items: [] });
    }
    const g = groups.find((g) => g.category === c.category && g.subcategory === c.subcategory);
    g!.items.push(c);
  }
  return groups;
}
