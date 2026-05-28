package cache

import (
	"html/template"
	"net/http"
	"path"
	"strconv"
	"time"
)

type uiPage struct {
	Name        string
	Version     string
	Root        string
	GeneratedAt time.Time
	FileCount   int
	TotalSize   int64
	TotalLabel  string
	Files       []uiFile
}

type uiFile struct {
	Kind     string
	Path     string
	URL      string
	Size     int64
	SizeText string
	ModTime  time.Time
	ModText  string
}

func (h cacheHandler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries, err := h.service.Scan(h.cacheDir, CacheScanOptions{
		Root:  h.opts.Root,
		Kinds: []Kind{KindPkg, KindAPI, KindSDK, KindSDKIndex},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	files := make([]uiFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if !pathStaysInDirAfterSymlinks(h.cacheDir, entry.Path) {
			continue
		}
		total += entry.Size
		files = append(files, uiFile{
			Kind:     string(entry.Kind),
			Path:     entry.RelPath,
			URL:      "/files/" + path.Clean(entry.RelPath),
			Size:     entry.Size,
			SizeText: formatBytes(entry.Size),
			ModTime:  entry.ModTime,
			ModText:  entry.ModTime.Format("2006-01-02 15:04:05"),
		})
	}

	root := h.opts.Root
	if root == "" {
		root = "all"
	}
	page := uiPage{
		Name:        "eget-cache",
		Version:     h.opts.Version,
		Root:        root,
		GeneratedAt: h.service.now(),
		FileCount:   len(files),
		TotalSize:   total,
		TotalLabel:  formatBytes(total),
		Files:       files,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if err := cacheIndexTemplate.Execute(w, page); err != nil {
		return
	}
}

func formatBytes(size int64) string {
	const unit = int64(1024)
	if size < unit {
		return strconv.FormatInt(size, 10) + " B"
	}
	div, exp := unit, 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	s := strconv.FormatFloat(float64(size)/float64(div), 'f', 1, 64)
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		s = s[:len(s)-2]
	}
	return s + " " + units[exp]
}

var cacheIndexTemplate = template.Must(template.New("cache-index").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Name}}</title>
<style>
:root{color-scheme:light;--bg:#f6f7f9;--panel:#fff;--ink:#20242a;--muted:#667085;--line:#d9dde5;--accent:#0f766e;--accent-ink:#fff;--chip:#eef4f3}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:14px/1.5 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
.wrap{max-width:1180px;margin:0 auto;padding:28px 20px 40px}
.top{display:flex;justify-content:space-between;gap:20px;align-items:flex-start;margin-bottom:22px}
h1{margin:0;font-size:28px;font-weight:750;letter-spacing:0}
.meta{margin-top:6px;color:var(--muted)}
.stats{display:grid;grid-template-columns:repeat(2,minmax(120px,1fr));gap:10px;min-width:280px}
.stat{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:12px 14px}
.stat b{display:block;font-size:20px}
.stat span{color:var(--muted);font-size:12px;text-transform:uppercase}
.tools{display:flex;gap:10px;margin:16px 0}
input,select{height:38px;border:1px solid var(--line);border-radius:7px;background:#fff;color:var(--ink);padding:0 12px}
input{flex:1;min-width:180px}
table{width:100%;border-collapse:separate;border-spacing:0;background:var(--panel);border:1px solid var(--line);border-radius:8px;overflow:hidden}
th,td{text-align:left;padding:10px 12px;border-bottom:1px solid var(--line);vertical-align:middle}
th{font-size:12px;color:var(--muted);text-transform:uppercase;background:#fafbfc}
tr:last-child td{border-bottom:0}
.kind{display:inline-flex;border-radius:999px;background:var(--chip);color:#115e59;padding:2px 8px;font-size:12px;font-weight:650}
.path{font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace;word-break:break-all}
.size,.time{white-space:nowrap;color:var(--muted)}
a.download{display:inline-flex;align-items:center;justify-content:center;height:30px;padding:0 10px;border-radius:7px;background:var(--accent);color:var(--accent-ink);text-decoration:none;font-weight:650}
.empty{padding:26px;text-align:center;color:var(--muted);background:var(--panel);border:1px solid var(--line);border-radius:8px}
@media (max-width:760px){.top{display:block}.stats{margin-top:16px;min-width:0}.tools{display:grid}th:nth-child(4),td:nth-child(4){display:none}}
</style>
</head>
<body>
<main class="wrap">
<section class="top">
<div>
<h1>{{.Name}}</h1>
<div class="meta">version: {{.Version}} · root: {{.Root}} · generated: {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</div>
</div>
<div class="stats">
<div class="stat"><b>{{.FileCount}} {{if eq .FileCount 1}}file{{else}}files{{end}}</b><span>Visible cache</span></div>
<div class="stat"><b>{{.TotalLabel}}</b><span>Total size</span></div>
</div>
</section>
<section class="tools">
<input id="search" type="search" placeholder="Search files" aria-label="Search files">
<select id="kind" aria-label="Kind">
<option value="">Kind</option>
<option value="pkg">pkg</option>
<option value="api">api</option>
<option value="sdk">sdk</option>
<option value="sdk-index">sdk-index</option>
</select>
</section>
{{if .Files}}
<table>
<thead><tr><th>Kind</th><th>Path</th><th>Size</th><th>Modified</th><th></th></tr></thead>
<tbody id="files">
{{range .Files}}
<tr data-kind="{{.Kind}}" data-path="{{.Path}}">
<td><span class="kind">{{.Kind}}</span></td>
<td class="path">{{.Path}}</td>
<td class="size">{{.SizeText}}</td>
<td class="time">{{.ModText}}</td>
<td><a class="download" href="{{.URL}}">Download</a></td>
</tr>
{{end}}
</tbody>
</table>
{{else}}
<div class="empty">No cache files match this scope.</div>
{{end}}
</main>
<script>
const search=document.getElementById('search');
const kind=document.getElementById('kind');
const rows=[...document.querySelectorAll('#files tr')];
function applyFilter(){
  const q=search.value.trim().toLowerCase();
  const k=kind.value;
  for(const row of rows){
    const okKind=!k||row.dataset.kind===k;
    const okPath=!q||row.dataset.path.toLowerCase().includes(q);
    row.hidden=!(okKind&&okPath);
  }
}
search.addEventListener('input',applyFilter);
kind.addEventListener('change',applyFilter);
</script>
</body>
</html>`))
