package distributed

const (
	DefaultBasePath = "/_simplecache/"
	DefaultReplicas = 50
)

type PeerPicker interface {
	PickPeer(key string) (PeerGetter, bool)
	GetAll() []string
}

type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}
