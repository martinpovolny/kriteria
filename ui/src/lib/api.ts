export interface Me {
  id: number;
  display_name: string;
  email: string;
  role: "teacher" | "director";
}

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
  deleted_at: string | null;
  deleted_by_name: string | null;
}

interface ApiOpts {
  // Skip the global redirect-to-/login on 401. Use for endpoints that have
  // their own auth flow (e.g. parent access), where a 401 just means "wrong
  // password" or "session expired", not "you're logged out of the app".
  skipAuthRedirect?: boolean;
}

async function api<T>(path: string, opts?: RequestInit & ApiOpts): Promise<T> {
  const { skipAuthRedirect, ...fetchOpts } = opts || {};
  const resp = await fetch(path, {
    ...fetchOpts,
    headers: { "Content-Type": "application/json", ...fetchOpts.headers },
  });
  if (resp.status === 401) {
    if (!skipAuthRedirect) {
      window.location.replace("/login");
      throw new Error("not authenticated");
    }
    const err = await resp.json().catch(() => ({ error: "unauthorized" }));
    throw new Error(err.error || "unauthorized");
  }
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: "request failed" }));
    throw new Error(err.error || `HTTP ${resp.status}`);
  }
  return resp.json();
}

export const apiGet = <T,>(path: string, opts?: ApiOpts) => api<T>(path, opts);
export const apiPost = <T,>(path: string, body?: any, opts?: ApiOpts) =>
  api<T>(path, { method: "POST", body: body ? JSON.stringify(body) : undefined, ...opts });
export const apiDelete = <T,>(path: string, opts?: ApiOpts) => api<T>(path, { method: "DELETE", ...opts });
