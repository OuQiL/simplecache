package distributed

import (
	"context"

	"github.com/OuQiL/simplecache/cachepb/cachepb"
)

const (
	DefaultBasePath = "/_simplecache/"
	DefaultReplicas = 50
)

type PeerPicker interface {
	PickPeer(key string) (PeerGetter, bool)
	GetAll() []string
}

type PeerGetter interface {
	Get(ctx context.Context, in *cachepb.GetRequest, out *cachepb.GetResponse) ([]byte, error)
}
