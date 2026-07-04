package api

import "net/http"

// accessPrintHandler serves a simple printable page: students grouped by
// class (grade), each showing name + URL + password. The director prints
// the whole page, cuts between class sections, and hands each section to
// the class teacher who further cuts and distributes to parents.
func accessPrintHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(accessPrintHTML))
}

const accessPrintHTML = `<!DOCTYPE html>
<html lang="cs">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Přístupové kódy — Kriteria</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, system-ui, "Segoe UI", Roboto, sans-serif;
         font-size: 14px; color: #1a1a2e; background: #f5f5f5; padding: 20px; }
  h1 { font-size: 22px; margin-bottom: 4px; }
  .subtitle { color: #666; margin-bottom: 16px; font-size: 13px; }
  .controls { margin-bottom: 16px; }
  .controls button { padding: 6px 14px; border: 1px solid #ccc; border-radius: 6px;
                     background: #fff; cursor: pointer; font-size: 13px; margin-right: 8px; }
  .controls button:hover { background: #e8e8e8; }

  .class-section { margin-bottom: 24px; page-break-after: always; }
  .class-section:last-child { page-break-after: auto; }
  .class-title { font-size: 18px; font-weight: 700; margin-bottom: 12px;
                 padding-bottom: 6px; border-bottom: 2px solid #333; }

  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
           gap: 12px; }
  .card { background: #fff; border: 2px dashed #ccc; border-radius: 8px;
          padding: 14px; page-break-inside: avoid; }
  .card .student { font-weight: 700; font-size: 16px; margin-bottom: 8px; }
  .card .url { font-family: monospace; font-size: 12px; color: #0066cc;
               word-break: break-all; margin-bottom: 4px; }
  .card .password { font-family: monospace; font-size: 15px; font-weight: 700;
                    padding: 4px 8px; background: #f0f0f0;
                    border-radius: 4px; text-align: center; letter-spacing: 1px; }
  .card .no-code { color: #999; font-size: 12px; font-style: italic; }

  @media print {
    body { background: #fff; padding: 0; }
    .controls { display: none; }
    .class-section { page-break-after: always; }
    .class-section:last-child { page-break-after: auto; }
  }
  .status-loading { padding: 40px; text-align: center; color: #666; }
</style>
</head>
<body>
<h1>Přístupové kódy pro rodiče</h1>
<p class="subtitle">Vytiskněte tuto stránku a rozstříhejte na lístečky pro rodiče</p>

<div class="controls">
  <button onclick="window.print()">Tisk</button>
</div>

<div id="content">
  <div class="status-loading">Načítám…</div>
</div>

<script>
async function load() {
  try {
    const resp = await fetch('/api/access-codes-by-class');
    const classes = await resp.json();
    render(classes);
  } catch (e) {
    document.getElementById('content').innerHTML =
      '<div class="status-loading">Chyba: ' + e.message + '</div>';
  }
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function render(classes) {
  const container = document.getElementById('content');
  container.innerHTML = '';

  if (!classes || classes.length === 0) {
    container.innerHTML = '<div class="status-loading">Žádné kódy</div>';
    return;
  }

  for (const cls of classes) {
    const section = document.createElement('div');
    section.className = 'class-section';

    let html = '<div class="class-title">' + cls.grade_level + '. ročník</div>';
    html += '<div class="cards">';

    for (const s of cls.students) {
      html += '<div class="card">';
      html += '<div class="student">' + escapeHtml(s.student_name) + '</div>';
      if (s.slug) {
        html += '<div class="url">hodnoceni.hmpf.cz/z/' + escapeHtml(s.slug) + '</div>';
        html += '<div class="password">' + escapeHtml(s.password || '—') + '</div>';
      } else {
        html += '<div class="no-code">bez přístupového kódu</div>';
      }
      html += '</div>';
    }

    html += '</div>';
    section.innerHTML = html;
    container.appendChild(section);
  }
}

load();
</script>
</body>
</html>`
