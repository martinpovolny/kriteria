export interface Subject {
  id: number;
  code: string;
  name: string;
  grades: { id: number; level: number }[];
}

export interface Grade {
  id: number;
  level: number;
}

export interface SchoolYear {
  id: number;
  label: string;
}

export interface Criterion {
  id: number;
  code: string;
  name: string;
  category: string;
  subcategory: string;
  ovu_code: string;
  sort_order: number;
  levels: {
    level: number;
    letter: string;
    label: string;
    description: string;
  }[];
}

export interface Student {
  id: number;
  display_name: string;
  created_at: string;
}

export interface Enrollment {
  id: number;
  student_id: number;
  subject_code: string;
  subject_name: string;
  grade_level: number;
  school_year: string;
}

export interface Evaluation {
  criterion_id: number;
  code: string;
  name: string;
  category: string;
  subcategory: string;
  current: {
    level: number;
    teacher_name: string;
    set_at: string;
    note: string;
  } | null;
  history: {
    level: number;
    teacher_name: string;
    set_at: string;
    note: string;
  }[];
}

export interface ParentAccess {
  id: number;
  slug: string;
  student_id: number;
  created_at: string;
  revoked_at: string;
  subject_code: string;
  subject_name: string;
  grade_level: number;
  school_year: string;
  url: string;
}

export interface DirectorStudent {
  id: number;
  display_name: string;
  created_at: string;
  enrollments: {
    subject_code: string;
    subject_name: string;
    grade_level: number;
    school_year: string;
  }[];
}

export interface AuditEntry {
  id: number;
  set_at: string;
  level: number;
  student_id: number;
  student_name: string;
  criterion_code: string;
  criterion_name: string;
  category: string;
  subcategory: string;
  subject_code: string;
  subject_name: string;
  grade_level: number;
  teacher_name: string;
  note: string;
}

async function api<T>(path: string, opts?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    ...opts,
    headers: { "Content-Type": "application/json", ...opts?.headers },
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: "request failed" }));
    throw new Error(err.error || `HTTP ${resp.status}`);
  }
  return resp.json();
}

export const apiGet = <T,>(path: string) => api<T>(path);
export const apiPost = <T,>(path: string, body?: any) =>
  api<T>(path, { method: "POST", body: body ? JSON.stringify(body) : undefined });
