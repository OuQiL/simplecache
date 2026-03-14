package distributed

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/OuQiL/simplecache/consistenthash"
)

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

	view, err := group.Get(key)
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

func (h *httpGetter) Get(group string, key string) ([]byte, error) {
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(group),
		url.QueryEscape(key),
	)

	res, err := http.Get(u)
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
