package main

import (
	"fmt"
	"os"
	"strings"
)

// Generates a standalone HTML file with the JSON data inlined,
// so it can be emailed/opened without a server.
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gen-overview <kriteria.json> [output.html]")
		os.Exit(1)
	}
	jsonPath := os.Args[1]
	outPath := "prehled.html"
	if len(os.Args) >= 3 {
		outPath = os.Args[2]
	}

	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read json:", err)
		os.Exit(1)
	}

	html := strings.Replace(overviewTemplate, "__DATA_PLACEHOLDER__", string(jsonData), 1)

	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write html:", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %s (%d bytes)\n", outPath, len(html))
}

const overviewTemplate = `<!DOCTYPE html>
<html lang="cs">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Přehled kritérií — Kriteria</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, system-ui, "Segoe UI", Roboto, sans-serif;
         font-size: 14px; line-height: 1.5; color: #1a1a2e; background: #f5f5f5; padding: 20px; }
  h1 { font-size: 22px; margin-bottom: 4px; }
  .subtitle { color: #666; margin-bottom: 20px; font-size: 13px; }
  .controls { margin-bottom: 16px; display: flex; gap: 8px; flex-wrap: wrap; }
  .controls button { padding: 6px 14px; border: 1px solid #ccc; border-radius: 6px;
                     background: #fff; cursor: pointer; font-size: 13px; }
  .controls button:hover { background: #e8e8e8; }
  .controls .count { margin-left: auto; color: #666; font-size: 13px; align-self: center; }

  .tree { background: #fff; border: 1px solid #ddd; border-radius: 8px; overflow: hidden; }
  .node { border-bottom: 1px solid #eee; }
  .node:last-child { border-bottom: none; }

  .row { display: flex; align-items: flex-start; gap: 6px; padding: 8px 12px; cursor: pointer;
         user-select: none; }
  .row:hover { background: #f0f4ff; }
  .row .toggle { width: 18px; height: 18px; flex-shrink: 0; display: inline-flex;
                 align-items: center; justify-content: center; color: #999; font-size: 12px;
                 transition: transform 0.15s; }
  .row.open > .toggle { transform: rotate(90deg); }
  .row.leaf > .toggle { visibility: hidden; }

  .row.grade    { font-weight: 600; font-size: 15px; background: #f8f8f8; }
  .row.subject  { font-weight: 600; font-size: 14px; }
  .row.criterion { font-size: 13px; }

  .badge { display: inline-block; padding: 1px 7px; border-radius: 10px; font-size: 11px;
           font-weight: 600; margin-left: 6px; }
  .badge.ok { background: #d4edda; color: #155724; }
  .badge.missing { background: #f8d7da; color: #721c24; }
  .badge.count { background: #e2e3e5; color: #383d41; }

  .children { display: none; }
  .children.open { display: block; }

  .criterion-detail { padding: 8px 12px 12px 36px; background: #fafafa; }
  .criterion-detail .ovu { color: #888; font-size: 12px; margin-bottom: 6px; }
  .criterion-detail .cat { color: #555; font-size: 12px; margin-bottom: 8px; }
  .criterion-detail .cat span { background: #e9ecef; padding: 1px 6px; border-radius: 4px; margin-right: 4px; }

  .levels { display: grid; grid-template-columns: 1fr; gap: 4px; margin-top: 6px; }
  .level { display: flex; gap: 8px; padding: 6px 10px; border-radius: 6px; font-size: 13px; }
  .level .letter { font-weight: 700; flex-shrink: 0; width: 22px; text-align: center; }
  .level .text { color: #333; }
  .level-4 { background: #d4edda; }
  .level-4 .letter { color: #155724; }
  .level-3 { background: #cce5ff; }
  .level-3 .letter { color: #004085; }
  .level-2 { background: #fff3cd; }
  .level-2 .letter { color: #856404; }
  .level-1 { background: #f8d7da; }
  .level-1 .letter { color: #721c24; }
</style>
</head>
<body>
<h1>Přehled kritérií</h1>
<p class="subtitle">Kritériové hodnocení žáků — kliknutím rozbalte ročník → předmět → kritérium</p>

<div class="controls">
  <button onclick="expandAll()">Rozbalit vše</button>
  <button onclick="collapseAll()">Sbalit vše</button>
  <button onclick="expandMissing()">Rozbalit jen chybějící</button>
  <span class="count" id="summary"></span>
</div>

<div id="tree" class="tree"></div>

<script>
const DATA = __DATA_PLACEHOLDER__;

function levelColorClass(level) { return 'level-' + level; }

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function render() {
  const container = document.getElementById('tree');
  container.innerHTML = '';

  const subjects = DATA.subjects || [];
  let totalCriteria = 0, totalMissing = 0;

  const gradesMap = {};
  for (const subj of subjects) {
    for (const grade of subj.grades || []) {
      if (!gradesMap[grade.level]) gradesMap[grade.level] = [];
      gradesMap[grade.level].push({ subject: subj, grade: grade });
    }
  }

  const sortedGrades = Object.keys(gradesMap).map(Number).sort((a,b) => a - b);

  for (const gradeLevel of sortedGrades) {
    const entries = gradesMap[gradeLevel];
    const allCriteria = entries.flatMap(e => e.grade.criteria || []);
    const missing = allCriteria.filter(c => !c.levels || c.levels.length === 0);
    totalCriteria += allCriteria.length;
    totalMissing += missing.length;

    const gradeEl = createNode(
      'ročník ' + gradeLevel,
      'grade',
      '<span class="badge count">' + allCriteria.length + ' kritérií</span>' +
      (missing.length > 0 ? '<span class="badge missing">' + missing.length + ' bez úrovní</span>' : '<span class="badge ok">ok</span>'),
      'grade-' + gradeLevel
    );
    const gradeChildren = document.createElement('div');
    gradeChildren.className = 'children';

    for (const entry of entries) {
      const subj = entry.subject;
      const grade = entry.grade;
      const criteria = grade.criteria || [];
      const subjMissing = criteria.filter(c => !c.levels || c.levels.length === 0);

      const subjEl = createNode(
        subj.code + ' — ' + subj.name,
        'subject',
        '<span class="badge count">' + criteria.length + '</span>' +
        (subjMissing.length > 0 ? '<span class="badge missing">' + subjMissing.length + ' chybí</span>' : '<span class="badge ok">ok</span>'),
        'subj-' + subj.code + '-' + gradeLevel
      );
      const subjChildren = document.createElement('div');
      subjChildren.className = 'children';

      for (const crit of criteria) {
        const hasLevels = crit.levels && crit.levels.length > 0;
        const critEl = createNode(
          '<b>' + escapeHtml(crit.code) + '</b> ' + escapeHtml(crit.name),
          'criterion ' + (hasLevels ? '' : 'leaf'),
          hasLevels ? '<span class="badge ok">' + crit.levels.length + ' úrovně</span>'
                    : '<span class="badge missing">bez úrovní</span>',
          null
        );

        if (hasLevels) {
          const detail = document.createElement('div');
          detail.className = 'children';

          let html = '<div class="criterion-detail">';
          if (crit.category || crit.subcategory) {
            html += '<div class="cat">';
            if (crit.category) html += '<span>' + escapeHtml(crit.category) + '</span>';
            if (crit.subcategory) html += '<span>' + escapeHtml(crit.subcategory) + '</span>';
            html += '</div>';
          }
          if (crit.ovu_code) {
            html += '<div class="ovu">OVU: ' + escapeHtml(crit.ovu_code) + '</div>';
          }
          html += '<div class="levels">';
          for (const lv of crit.levels) {
            html += '<div class="level ' + levelColorClass(lv.level) + '">' +
              '<span class="letter">' + escapeHtml(lv.letter) + '</span>' +
              '<span class="text"><b>' + escapeHtml(lv.label) + '</b>: ' +
              escapeHtml(lv.description) + '</span></div>';
          }
          html += '</div></div>';
          detail.innerHTML = html;
          critEl.querySelector('.row').after(detail);
          critEl._children = detail;
        }

        subjChildren.appendChild(critEl);
      }

      subjEl.querySelector('.row').after(subjChildren);
      subjEl._children = subjChildren;
      gradeChildren.appendChild(subjEl);
    }

    gradeEl.querySelector('.row').after(gradeChildren);
    gradeEl._children = gradeChildren;
    container.appendChild(gradeEl);
  }

  document.getElementById('summary').textContent =
    totalCriteria + ' kritérií celkem, ' + totalMissing + ' bez úrovní';
}

function createNode(label, classes, badges, id) {
  const el = document.createElement('div');
  el.className = 'node';
  if (id) el.id = id;

  const row = document.createElement('div');
  row.className = 'row ' + classes;
  row.innerHTML = '<span class="toggle">&#9654;</span><span class="label">' + label + '</span>' + badges;
  row.addEventListener('click', function(e) {
    e.stopPropagation();
    toggleNode(el);
  });
  el.appendChild(row);
  return el;
}

function toggleNode(el) {
  if (!el._children) return;
  const row = el.querySelector('.row');
  const open = row.classList.toggle('open');
  el._children.classList.toggle('open', open);
}

function expandAll() {
  document.querySelectorAll('.node .row').forEach(function(r) {
    const node = r.closest('.node');
    if (node._children) {
      r.classList.add('open');
      node._children.classList.add('open');
    }
  });
}

function collapseAll() {
  document.querySelectorAll('.node .row.open').forEach(function(r) {
    const node = r.closest('.node');
    if (node._children) {
      r.classList.remove('open');
      node._children.classList.remove('open');
    }
  });
}

function expandMissing() {
  collapseAll();
  document.querySelectorAll('.node').forEach(function(node) {
    const badges = node.querySelectorAll('.badge.missing');
    if (badges.length > 0) {
      const row = node.querySelector(':scope > .row');
      if (row && node._children) {
        row.classList.add('open');
        node._children.classList.add('open');
      }
    }
  });
}

render();
</script>
</body>
</html>`
