package app

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"
	"golang.org/x/crypto/bcrypt"
	"portbridge/internal/store"
	"portbridge/internal/telemetry"
	"portbridge/internal/transport"
	"portbridge/internal/web"
)

type liveNode struct {
	session *yamux.Session
	status  telemetry.Status
	seen    time.Time
}

type attempt struct {
	at    time.Time
	count int
}

type Hub struct {
	proxies     map[string]*liveProxy
	Config      Config
	Store       *store.Store
	Fingerprint string
	mu          sync.Mutex
	live        map[string]*liveNode
	attempts    map[string]attempt
	ctx         context.Context
	closed      bool
	dir         string
	stopMonitor chan struct{}
	monitorDone chan struct{}
}

func NewHub(ctx context.Context, dir string, c Config) (*Hub, error) {
	s, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	// Fingerprint is read from the validated certificate/key pair.
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		s.Close()
		return nil, err
	}
	fp := hashBytes(cert.Certificate[0])
	h := &Hub{proxies: map[string]*liveProxy{}, Config: c, Store: s, Fingerprint: fp, live: map[string]*liveNode{}, attempts: map[string]attempt{}, ctx: ctx, dir: dir, stopMonitor: make(chan struct{}), monitorDone: make(chan struct{})}
	err = s.Update(func(state *store.State) error {
		if len(state.Users) > 0 {
			return nil
		}
		password := randomToken()[:24]
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, "initial-admin.txt"), []byte("username: admin\npassword: "+password+"\n"), 0600); err != nil {
			return err
		}
		state.Users["admin"] = store.User{Username: "admin", Password: string(hashed), Admin: true, Created: time.Now().UTC()}
		state.Record("system", "bootstrap", "admin")
		return nil
	})
	if err != nil {
		s.Close()
		return nil, err
	}
	var nodes []store.Node
	var proxies []store.Proxy
	if err = s.View(func(st store.State) error {
		for _, p := range st.Proxies {
			proxies = append(proxies, p)
		}
		for _, n := range st.Nodes {
			nodes = append(nodes, n)
		}
		return nil
	}); err != nil {
		s.Close()
		return nil, err
	}
	for _, n := range nodes {
		live := &liveNode{}
		h.mu.Lock()
		h.live[n.ID] = live
		h.mu.Unlock()
	}
	for _, p := range proxies {
		h.restoreProxy(p)
	}
	go h.monitor()
	return h, nil
}

func (h *Hub) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	close(h.stopMonitor)
	for _, l := range h.live {
		if l.session != nil {
			l.session.Close()
		}
	}
	for _, p := range h.proxies {
		p.close()
	}
	h.mu.Unlock()
	<-h.monitorDone
	return h.Store.Close()
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 415, "请发送 JSON 请求")
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "请求数据无效")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "请求数据无效")
		return false
	}
	return true
}

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" {
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					fail(w, 403, "请求来源不允许")
					return
				}
			}
			if r.URL.Path != "/api/enroll" && r.Header.Get("X-PortBridge-Request") != "1" {
				fail(w, 403, "缺少请求校验头")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"ok": true, "mode": "hub"})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		var allow bool
		if err := h.Store.View(func(s store.State) error { allow = s.AllowRegister; return nil }); err != nil {
			fail(w, 500, "读取配置失败")
			return
		}
		respond(w, 200, map[string]any{"mode": "hub", "allow_register": allow})
	})
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/register", h.register)
	mux.HandleFunc("POST /api/enroll", h.enroll)
	mux.HandleFunc("GET /api/connect", h.connect)
	mux.Handle("GET /api/me", h.auth(false, func(w http.ResponseWriter, r *http.Request, u store.User) { u.Password = ""; respond(w, 200, u) }))
	mux.Handle("POST /api/logout", h.auth(false, h.logout))
	mux.Handle("PUT /api/password", h.auth(false, h.password))
	mux.Handle("GET /api/nodes", h.auth(false, h.nodes))
	mux.Handle("GET /api/proxies", h.auth(true, h.listProxies))
	mux.Handle("POST /api/proxies", h.auth(true, h.createProxy))
	mux.Handle("PUT /api/proxies/{id}/forwarding", h.auth(true, h.pauseProxy))
	mux.Handle("DELETE /api/proxies/{id}", h.auth(true, h.deleteProxy))
	mux.Handle("POST /api/invites", h.auth(true, h.invite))
	mux.Handle("DELETE /api/nodes/{id}", h.auth(true, h.deleteNode))
	mux.Handle("GET /api/users", h.auth(true, h.users))
	mux.Handle("POST /api/users", h.auth(true, h.createUser))
	mux.Handle("PATCH /api/users/{username}", h.auth(true, h.editUser))
	mux.Handle("DELETE /api/users/{username}", h.auth(true, h.deleteUser))
	mux.Handle("PUT /api/settings", h.auth(true, h.settings))
	mux.Handle("GET /api/audit", h.auth(true, func(w http.ResponseWriter, r *http.Request, u store.User) {
		var a []store.Audit
		if err := h.Store.View(func(s store.State) error { a = s.Audit; return nil }); err != nil {
			fail(w, 500, "读取审计失败")
			return
		}
		respond(w, 200, a)
	}))
	mux.Handle("/", web.Handler())
	return security(mux)
}

func (h *Hub) auth(admin bool, fn func(http.ResponseWriter, *http.Request, store.User)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("portbridge_session")
		if err != nil {
			fail(w, 401, "请先登录")
			return
		}
		var user store.User
		var valid bool
		err = h.Store.View(func(s store.State) error {
			session, ok := s.Sessions[hashToken(c.Value)]
			if !ok || time.Now().After(session.Expires) {
				return nil
			}
			user, valid = s.Users[session.Username]
			return nil
		})
		if err != nil {
			fail(w, 500, "读取账号失败")
			return
		}
		if !valid {
			fail(w, 401, "登录已过期")
			return
		}
		if admin && !user.Admin {
			slog.Warn("permission denied", "user", user.Username, "path", r.URL.Path)
			fail(w, 403, "需要管理员权限")
			return
		}
		fn(w, r, user)
	})
}

func (h *Hub) limited(r *http.Request) bool {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for k, v := range h.attempts {
		if now.Sub(v.at) > time.Minute {
			delete(h.attempts, k)
		}
	}
	a := h.attempts[ip]
	if a.at.IsZero() {
		a.at = now
	}
	a.count++
	if len(h.attempts) >= 4096 {
		return true
	}
	h.attempts[ip] = a
	return a.count > 20
}

func (h *Hub) nodes(w http.ResponseWriter, r *http.Request, u store.User) {
	var saved []store.Node
	if err := h.Store.View(func(s store.State) error {
		for _, n := range s.Nodes {
			n.CredentialHash = ""
			saved = append(saved, n)
		}
		return nil
	}); err != nil {
		fail(w, 500, "读取节点失败")
		return
	}
	sort.Slice(saved, func(i, j int) bool { return saved[i].Created.Before(saved[j].Created) })
	result := []map[string]any{}
	h.mu.Lock()
	for _, n := range saved {
		l := h.live[n.ID]
		row := map[string]any{"id": n.ID, "name": n.Name, "online": h.nodeOnline(l), "last_seen": n.LastSeen}
		if l != nil {
			row["status"] = l.status
			if !l.seen.IsZero() {
				row["last_seen"] = l.seen
			}
		}
		result = append(result, row)
	}
	h.mu.Unlock()
	respond(w, 200, result)
}

func (h *Hub) invite(w http.ResponseWriter, r *http.Request, u store.User) {
	var p struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &p) {
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 100 {
		fail(w, 400, "节点名称应为 1–100 字符")
		return
	}
	token := randomToken()
	expires := time.Now().Add(15 * time.Minute)
	err := h.Store.Update(func(s *store.State) error {
		for k, v := range s.Invites {
			if time.Now().After(v.Expires) {
				delete(s.Invites, k)
			}
		}
		if len(s.Invites) >= 100 {
			return errors.New("待配对配置过多，请稍后再试")
		}
		s.Invites[hashToken(token)] = store.Invite{Name: p.Name, Expires: expires}
		s.Record(u.Username, "invite.create", p.Name)
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	respond(w, 201, map[string]any{"link": EncodePair(Pairing{h.Config.PublicURL, token, h.Fingerprint}), "expires": expires})
}

func (h *Hub) enroll(w http.ResponseWriter, r *http.Request) {
	if h.limited(r) {
		fail(w, 429, "请求过于频繁，请稍后再试")
		return
	}
	var p struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &p) {
		return
	}
	credential, id := randomToken(), randomToken()[:16]
	var n store.Node
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		fail(w, 503, "控制台正在停止")
		return
	}
	err := h.Store.Update(func(s *store.State) error {
		key := hashToken(p.Token)
		invite, ok := s.Invites[key]
		if !ok || time.Now().After(invite.Expires) {
			return errors.New("配对链接已使用或已过期，请在控制台重新生成")
		}
		if len(s.Nodes) >= 100 {
			return errors.New("最多接入 100 个节点")
		}
		n = store.Node{ID: id, Name: invite.Name, CredentialHash: hashToken(credential), Created: time.Now().UTC()}
		s.Nodes[id] = n
		delete(s.Invites, key)
		s.Record("node", "node.enroll", id)
		return nil
	})
	if err == nil {
		h.live[id] = &liveNode{}
	}
	h.mu.Unlock()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	respond(w, 201, map[string]any{"node_id": n.ID, "credential": credential, "name": n.Name})
}

func (h *Hub) deleteNode(w http.ResponseWriter, r *http.Request, u store.User) {
	id := r.PathValue("id")
	h.mu.Lock()
	err := h.Store.Update(func(s *store.State) error {
		if _, ok := s.Nodes[id]; !ok {
			return errors.New("节点不存在")
		}
		delete(s.Nodes, id)
		for proxyID, p := range s.Proxies {
			if p.NodeID == id {
				delete(s.Proxies, proxyID)
			}
		}
		s.Record(u.Username, "node.delete", id)
		return nil
	})
	if err == nil {
		if l := h.live[id]; l != nil {
			if l.session != nil {
				l.session.Close()
			}
		}
		delete(h.live, id)
		for proxyID, p := range h.proxies {
			if p.saved.NodeID == id {
				p.close()
				delete(h.proxies, proxyID)
			}
		}
	}
	h.mu.Unlock()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}

func (h *Hub) connect(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		fail(w, 403, "节点连接不接受浏览器请求")
		return
	}
	id := r.Header.Get("X-Node-ID")
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var valid bool
	if err := h.Store.View(func(s store.State) error {
		n, ok := s.Nodes[id]
		valid = ok && subtle.ConstantTimeCompare([]byte(n.CredentialHash), []byte(hashToken(token))) == 1
		return nil
	}); err != nil {
		fail(w, 500, "读取节点失败")
		return
	}
	if !valid {
		fail(w, 401, "节点凭证无效")
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn := websocket.NetConn(h.ctx, ws, websocket.MessageBinary)
	session, err := yamux.Server(conn, transport.Config())
	if err != nil {
		conn.Close()
		return
	}
	defer session.Close()
	h.mu.Lock()
	live := h.live[id]
	if h.closed || live == nil {
		h.mu.Unlock()
		return
	}
	if live.session != nil {
		live.session.Close()
	}
	live.session = session
	live.seen = time.Time{}
	h.mu.Unlock()
	slog.Info("node connected", "node", id)
	defer func() {
		h.mu.Lock()
		if l := h.live[id]; l != nil && l.session == session {
			l.session = nil
		}
		h.mu.Unlock()
		slog.Info("node disconnected", "node", id)
	}()
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return
		}
		// Only telemetry flows node -> hub. Bound each report and its lifetime.
		stream.SetDeadline(time.Now().Add(5 * time.Second))
		var kind [1]byte
		if _, err = io.ReadFull(stream, kind[:]); err == nil && kind[0] == transport.Status {
			var status telemetry.Status
			if json.NewDecoder(io.LimitReader(stream, 64*1024)).Decode(&status) == nil {
				h.mu.Lock()
				if l := h.live[id]; l != nil && l.session == session {
					l.status = status
					l.seen = time.Now().UTC()
				}
				h.mu.Unlock()
			}
		}
		stream.Close()
	}
}

func HTTPServer(listen string, handler http.Handler) *http.Server {
	return &http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 75 * time.Second, MaxHeaderBytes: 16 * 1024}
}

func InitHub(dir, listen, publicURL string, start, end int) (Config, error) {
	u, err := validateURL(publicURL)
	if err != nil {
		return Config{}, err
	}
	if start < 1024 || end > 65535 || start > end {
		return Config{}, errors.New("公网端口池应在 1024–65535 之间")
	}
	if _, _, err = net.SplitHostPort(listen); err != nil {
		return Config{}, fmt.Errorf("监听地址无效: %w", err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Config{}, err
	}
	cert, key, _, err := certificate(dir, u.Hostname())
	if err != nil {
		return Config{}, err
	}
	c := Config{Project: "portbridge", Mode: "hub", Listen: listen, PublicURL: strings.TrimRight(publicURL, "/"), PortStart: start, PortEnd: end, CertFile: cert, KeyFile: key}
	return c, WriteJSON(filepath.Join(dir, "config.json"), c)
}
