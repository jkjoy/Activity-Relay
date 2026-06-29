package api

import (
	"crypto/subtle"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/viper"
	"github.com/yukimochi/Activity-Relay/models"
)

// ServerRow is the public/admin presentation model for a relay member or domain rule.
type ServerRow struct {
	Domain         string `json:"domain"`
	InboxURL       string `json:"inbox_url,omitempty"`
	ActorID        string `json:"actor_id,omitempty"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	MutuallyFollow bool   `json:"mutually_follow,omitempty"`
}

// ServerListResponse is returned by /api/servers.
type ServerListResponse struct {
	Total   int         `json:"total"`
	Servers []ServerRow `json:"servers"`
}

func buildServerRows() []ServerRow {
	state := models.NewState(RelayState.RedisClient, false)
	limited := stringSet(state.LimitedDomains)
	blocked := stringSet(state.BlockedDomains)
	seen := map[string]bool{}
	rows := []ServerRow{}

	for _, subscriber := range state.Subscribers {
		status := "active"
		if limited[subscriber.Domain] {
			status = "limited"
		}
		if blocked[subscriber.Domain] {
			status = "blocked"
		}
		rows = append(rows, ServerRow{
			Domain:   subscriber.Domain,
			InboxURL: subscriber.InboxURL,
			ActorID:  subscriber.ActorID,
			Kind:     "subscriber",
			Status:   status,
		})
		seen[subscriber.Domain] = true
	}

	for _, follower := range state.Followers {
		status := "active"
		if limited[follower.Domain] {
			status = "limited"
		}
		if blocked[follower.Domain] {
			status = "blocked"
		}
		kind := "follower"
		if existing := findRow(rows, follower.Domain); existing != nil {
			existing.Kind = "subscriber+follower"
			existing.MutuallyFollow = follower.MutuallyFollow
			seen[follower.Domain] = true
			continue
		}
		rows = append(rows, ServerRow{
			Domain:         follower.Domain,
			InboxURL:       follower.InboxURL,
			ActorID:        follower.ActorID,
			Kind:           kind,
			Status:         status,
			MutuallyFollow: follower.MutuallyFollow,
		})
		seen[follower.Domain] = true
	}

	for _, domain := range state.BlockedDomains {
		if !seen[domain] {
			rows = append(rows, ServerRow{Domain: domain, Kind: "rule", Status: "blocked"})
		}
	}
	for _, domain := range state.LimitedDomains {
		if !seen[domain] && !blocked[domain] {
			rows = append(rows, ServerRow{Domain: domain, Kind: "rule", Status: "limited"})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Domain < rows[j].Domain })
	return rows
}

func stringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	return set
}

func findRow(rows []ServerRow, domain string) *ServerRow {
	for index := range rows {
		if rows[index].Domain == domain {
			return &rows[index]
		}
	}
	return nil
}

func handleAPIServers(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rows := buildServerRows()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	json.NewEncoder(writer).Encode(ServerListResponse{Total: len(rows), Servers: rows})
}

func handlePublicIndex(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	renderHTML(writer, publicIndexTemplate, map[string]interface{}{
		"ServiceName": GlobalConfig.ServerServiceName(),
		"InboxURL":    "https://" + GlobalConfig.ServerHostname().Host + "/inbox",
		"ActorURL":    "https://" + GlobalConfig.ServerHostname().Host + "/actor",
		"Total":       len(buildServerRows()),
	})
}

func handlePublicServers(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	renderHTML(writer, serverListTemplate, map[string]interface{}{
		"ServiceName": GlobalConfig.ServerServiceName(),
		"Servers":     buildServerRows(),
	})
}

func handleAdminIndex(writer http.ResponseWriter, request *http.Request) {
	if !requireAdmin(writer, request) {
		return
	}
	renderHTML(writer, adminServersTemplate, map[string]interface{}{
		"ServiceName": GlobalConfig.ServerServiceName(),
		"Servers":     buildServerRows(),
	})
}

func handleAdminServerAction(writer http.ResponseWriter, request *http.Request) {
	switch {
	case strings.HasSuffix(request.URL.Path, "/delete"):
		handleAdminDeleteServer(writer, request)
	case strings.HasSuffix(request.URL.Path, "/limit"):
		handleAdminLimitServer(writer, request)
	case strings.HasSuffix(request.URL.Path, "/unlimit"):
		handleAdminUnlimitServer(writer, request)
	case strings.HasSuffix(request.URL.Path, "/block"):
		handleAdminBlockServer(writer, request)
	case strings.HasSuffix(request.URL.Path, "/unblock"):
		handleAdminUnblockServer(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

func handleAdminDeleteServer(writer http.ResponseWriter, request *http.Request) {
	if !requireAdmin(writer, request) {
		return
	}
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	domain := domainFromAdminPath(request.URL.Path, "/admin/servers/", "/delete")
	if domain == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	deleteMembership(domain)
	redirectAdminServers(writer, request)
}

func handleAdminLimitServer(writer http.ResponseWriter, request *http.Request) {
	if !requireAdmin(writer, request) {
		return
	}
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	domain := domainFromAdminPath(request.URL.Path, "/admin/servers/", "/limit")
	if domain == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	RelayState.SetLimitedDomain(domain, true)
	redirectAdminServers(writer, request)
}

func handleAdminUnlimitServer(writer http.ResponseWriter, request *http.Request) {
	if !requireAdmin(writer, request) {
		return
	}
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	domain := domainFromAdminPath(request.URL.Path, "/admin/servers/", "/unlimit")
	if domain == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	RelayState.SetLimitedDomain(domain, false)
	redirectAdminServers(writer, request)
}

func handleAdminBlockServer(writer http.ResponseWriter, request *http.Request) {
	if !requireAdmin(writer, request) {
		return
	}
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	domain := domainFromAdminPath(request.URL.Path, "/admin/servers/", "/block")
	if domain == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	deleteMembership(domain)
	RelayState.SetBlockedDomain(domain, true)
	redirectAdminServers(writer, request)
}

func handleAdminUnblockServer(writer http.ResponseWriter, request *http.Request) {
	if !requireAdmin(writer, request) {
		return
	}
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	domain := domainFromAdminPath(request.URL.Path, "/admin/servers/", "/unblock")
	if domain == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	RelayState.SetBlockedDomain(domain, false)
	redirectAdminServers(writer, request)
}

func deleteMembership(domain string) {
	if RelayState.SelectSubscriber(domain) != nil {
		RelayState.DelSubscriber(domain)
	}
	if RelayState.SelectFollower(domain) != nil {
		RelayState.DelFollower(domain)
	}
}

func domainFromAdminPath(path string, prefix string, suffix string) string {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	domain := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	domain = strings.Trim(domain, "/ ")
	if strings.Contains(domain, "/") {
		return ""
	}
	return domain
}

func requireAdmin(writer http.ResponseWriter, request *http.Request) bool {
	if !viper.GetBool("ADMIN_ENABLED") {
		http.NotFound(writer, request)
		return false
	}
	username := viper.GetString("ADMIN_USERNAME")
	password := viper.GetString("ADMIN_PASSWORD")
	if username == "" || password == "" {
		writer.WriteHeader(http.StatusServiceUnavailable)
		writer.Write([]byte("admin is enabled but ADMIN_USERNAME or ADMIN_PASSWORD is empty"))
		return false
	}
	providedUser, providedPassword, ok := request.BasicAuth()
	if ok && secureCompare(providedUser, username) && secureCompare(providedPassword, password) {
		if !validAdminRequestOrigin(request) {
			writer.WriteHeader(http.StatusForbidden)
			return false
		}
		return true
	}
	writer.Header().Set("WWW-Authenticate", `Basic realm="Activity-Relay Admin"`)
	writer.WriteHeader(http.StatusUnauthorized)
	return false
}

func validAdminRequestOrigin(request *http.Request) bool {
	if request.Method == http.MethodGet || request.Method == http.MethodHead || request.Method == http.MethodOptions {
		return true
	}
	if origin := request.Header.Get("Origin"); origin != "" {
		return sameHost(origin, request.Host)
	}
	if referer := request.Header.Get("Referer"); referer != "" {
		return sameHost(referer, request.Host)
	}
	return true
}

func sameHost(rawURL string, requestHost string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, requestHost)
}

func secureCompare(a string, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func redirectAdminServers(writer http.ResponseWriter, request *http.Request) {
	http.Redirect(writer, request, "/admin/servers", http.StatusSeeOther)
}

func renderHTML(writer http.ResponseWriter, tmpl string, data interface{}) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	parsed := template.Must(template.New("page").Parse(tmpl))
	parsed.Execute(writer, data)
}

const publicIndexTemplate = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>{{.ServiceName}}</title><style>` + pageCSS + `</style></head>
<body><main class="container"><h1>{{.ServiceName}}</h1><p>这是一个 ActivityPub 联邦宇宙中继服务。</p><section class="card"><h2>加入中继</h2><p>Mastodon / Misskey 可订阅 Inbox：</p><code>{{.InboxURL}}</code><p>Pleroma / Akkoma 可 Follow Actor：</p><code>{{.ActorURL}}</code></section><section class="card"><h2>中继列表</h2><p>当前记录 {{.Total}} 个实例或规则。</p><p><a class="button" href="/servers">查看中继列表</a> <a class="button" href="/api/servers">JSON API</a></p></section></main></body></html>`

const serverListTemplate = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>{{.ServiceName}} - 中继列表</title><style>` + pageCSS + `</style></head>
<body><main class="container"><h1>中继列表</h1><p><a href="/">返回首页</a></p><table><thead><tr><th>域名</th><th>类型</th><th>状态</th><th>Inbox</th></tr></thead><tbody>{{range .Servers}}<tr><td>{{.Domain}}</td><td>{{.Kind}}</td><td><span class="badge {{.Status}}">{{.Status}}</span></td><td>{{.InboxURL}}</td></tr>{{else}}<tr><td colspan="4">暂无实例</td></tr>{{end}}</tbody></table></main></body></html>`

const adminServersTemplate = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>{{.ServiceName}} - 后台</title><style>` + pageCSS + `</style></head>
<body><main class="container"><h1>中继后台</h1><p><a href="/">首页</a> · <a href="/servers">公开列表</a></p><table><thead><tr><th>域名</th><th>类型</th><th>状态</th><th>操作</th></tr></thead><tbody>{{range .Servers}}<tr><td>{{.Domain}}</td><td>{{.Kind}}</td><td><span class="badge {{.Status}}">{{.Status}}</span></td><td class="actions"><form method="post" action="/admin/servers/{{.Domain}}/limit"><button>限制</button></form><form method="post" action="/admin/servers/{{.Domain}}/unlimit"><button>解除限制</button></form><form method="post" action="/admin/servers/{{.Domain}}/block"><button>拉黑</button></form><form method="post" action="/admin/servers/{{.Domain}}/unblock"><button>解除拉黑</button></form><form method="post" action="/admin/servers/{{.Domain}}/delete"><button class="danger">删除</button></form></td></tr>{{else}}<tr><td colspan="4">暂无实例</td></tr>{{end}}</tbody></table></main></body></html>`

const pageCSS = `body{font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f6f7fb;color:#172033;margin:0}.container{max-width:1080px;margin:0 auto;padding:32px}.card{background:white;border:1px solid #e5e7eb;border-radius:14px;padding:20px;margin:18px 0;box-shadow:0 1px 3px #0001}code{display:block;background:#111827;color:#f9fafb;border-radius:10px;padding:12px;overflow:auto}a{color:#2563eb}.button,button{display:inline-block;border:0;border-radius:8px;background:#2563eb;color:white;padding:8px 12px;text-decoration:none;cursor:pointer}button.danger{background:#dc2626}table{width:100%;border-collapse:collapse;background:white;border-radius:14px;overflow:hidden}th,td{border-bottom:1px solid #e5e7eb;padding:10px;text-align:left;vertical-align:top}.badge{border-radius:999px;padding:3px 8px;background:#e5e7eb}.badge.active{background:#dcfce7;color:#166534}.badge.limited{background:#fef3c7;color:#92400e}.badge.blocked{background:#fee2e2;color:#991b1b}.actions{display:flex;gap:6px;flex-wrap:wrap}.actions form{margin:0}`
