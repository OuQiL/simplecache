package distributed

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/OuQiL/simplecache/cachepb"
	"github.com/OuQiL/simplecache/consistenthash"
)

// 全局HTTP客户端，带连接池
var httpClient = &http.Client{
	Transport: &http.Transport{
		// 连接池配置
		MaxIdleConns:        100, // 最大空闲连接数
		MaxIdleConnsPerHost: 10,  // 每个主机的最大空闲连接数
		MaxConnsPerHost:     20,  // 每个主机的最大连接数

		// 超时配置
		IdleConnTimeout:     30 * time.Second, // 空闲连接超时时间
		TLSHandshakeTimeout: 5 * time.Second,  // TLS握手超时

		// 连接建立配置
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,  // 连接建立超时
			KeepAlive: 30 * time.Second, // TCP keepalive
		}).DialContext,

		// 启用HTTP/2
		ForceAttemptHTTP2: true,
	},
}

type HTTPPool struct {
	sync.RWMutex
	self        string
	basePath    string
	peers       *consistenthash.Map
	httpGetters map[string]*httpGetter
}

func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: DefaultBasePath,
		peers:    consistenthash.New(DefaultReplicas, nil),
	}
}

func (p *HTTPPool) Set(peers ...string) {
	p.Lock()
	defer p.Unlock()

	p.peers = consistenthash.New(DefaultReplicas, nil)
	p.peers.Add(peers...)
	p.httpGetters = make(map[string]*httpGetter, len(peers))
	for _, peer := range peers {
		p.httpGetters[peer] = &httpGetter{baseURL: peer + p.basePath}
	}
}

func (p *HTTPPool) PickPeer(key string) (PeerGetter, bool) {
	p.RLock()
	defer p.RUnlock()

	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		return p.httpGetters[peer], true
	}
	return nil, false
}

func (p *HTTPPool) GetAll() []string {
	p.RLock()
	defer p.RUnlock()

	var peers []string
	for k := range p.httpGetters {
		peers = append(peers, k)
	}
	return peers
}

func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, p.basePath) {
		http.Error(w, "HTTPPool serving unexpected path: "+r.URL.Path, http.StatusNotFound)
		return
	}

	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	groupName := parts[0]
	key := parts[1]

	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}

	view, err := group.Get(context.Background(), key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(view.ByteSlice())
}

type httpGetter struct {
	baseURL string
}

func (h *httpGetter) Get(ctx context.Context, in *cachepb.GetRequest, out *cachepb.GetResponse) ([]byte, error) {
	group := in.Group
	key := in.Key
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(group),
		url.QueryEscape(key),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", res.Status)
	}

	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}

	return bytes, nil
}

var _ PeerGetter = (*httpGetter)(nil)
