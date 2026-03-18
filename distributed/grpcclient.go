package distributed

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/OuQiL/simplecache/cachepb"
	"github.com/OuQiL/simplecache/consistenthash"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type GrpcClient struct {
	*Group
	registry    *EtcdRegister
	hashRing    *consistenthash.Map
	connections sync.Map
	mu          sync.RWMutex

	connTimeout   time.Duration
	keepAliveTime time.Duration
}

type GrpcClientConfig struct {
	GroupName       string
	GroupCacheBytes int64
	GroupTimeout    time.Duration
	Getter          Getter
	EtcdEndpoints   []string
	ConnTimeout     time.Duration
	Replicas        int
}

func NewGrpcClient(cfg GrpcClientConfig) (*GrpcClient, error) {
	reg, err := NewEtcdRegister(cfg.EtcdEndpoints, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry: %w", err)
	}

	if cfg.ConnTimeout == 0 {
		cfg.ConnTimeout = 5 * time.Second
	}
	if cfg.Replicas == 0 {
		cfg.Replicas = 50
	}

	group := NewGroup(
		cfg.GroupName,
		cfg.GroupCacheBytes,
		cfg.GroupTimeout,
		cfg.Getter,
	)

	client := &GrpcClient{
		Group:         group,
		registry:      reg,
		hashRing:      consistenthash.New(cfg.Replicas, nil),
		connTimeout:   cfg.ConnTimeout,
		keepAliveTime: 10 * time.Second,
	}

	client.Group.RegisterPeers(client)

	go client.discoverServices()

	return client, nil
}

func (c *GrpcClient) discoverServices() {
	ctx := context.Background()
	servicePrefix := fmt.Sprintf("/simplecache/%s/nodes/", c.Group.Name())

	nodes, err := c.registry.Discover(ctx, servicePrefix)
	if err != nil {
		log.Printf("Initial discovery failed: %v", err)
	} else {
		c.updateNodes(nodes)
	}

	go c.registry.Watch(ctx, servicePrefix, func(node NodeInfo, added bool) {
		if added {
			c.addNode(node)
		} else {
			c.removeNode(node)
		}
	})
}

func (c *GrpcClient) updateNodes(nodes []NodeInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var addrs []string
	for _, node := range nodes {
		addrs = append(addrs, node.nodeAddr)
	}

	c.hashRing = consistenthash.New(50, nil)
	c.hashRing.Add(addrs...)
	log.Printf("Updated nodes: %v", addrs)
}

func (c *GrpcClient) addNode(node NodeInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashRing.Add(node.nodeAddr)
	log.Printf("Node added: %s", node.nodeAddr)
}

func (c *GrpcClient) removeNode(node NodeInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashRing.Remove(node.nodeAddr)

	if conn, ok := c.connections.Load(node.nodeAddr); ok {
		conn.(*grpc.ClientConn).Close()
		c.connections.Delete(node.nodeAddr)
	}
	log.Printf("Node removed: %s", node.nodeAddr)
}

func (c *GrpcClient) getConnection(addr string) (pb.CacheServiceClient, error) {
	if conn, ok := c.connections.Load(addr); ok {
		return pb.NewCacheServiceClient(conn.(*grpc.ClientConn)), nil
	}

	kaParams := keepalive.ClientParameters{
		Time:    c.keepAliveTime,
		Timeout: time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.connTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kaParams),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	c.connections.Store(addr, conn)
	return pb.NewCacheServiceClient(conn), nil
}

func (c *GrpcClient) PickPeer(key string) (PeerGetter, bool) {
	c.mu.RLock()
	addr := c.hashRing.Get(key)
	c.mu.RUnlock()

	if addr == "" {
		return nil, false
	}

	client, err := c.getConnection(addr)
	if err != nil {
		return nil, false
	}

	return &grpcPeerGetter{client: client, group: c.Group.Name()}, true
}

func (c *GrpcClient) GetAll() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var peers []string
	c.connections.Range(func(key, value interface{}) bool {
		peers = append(peers, key.(string))
		return true
	})
	return peers
}

func (c *GrpcClient) Close() error {
	c.connections.Range(func(key, value interface{}) bool {
		value.(*grpc.ClientConn).Close()
		return true
	})
	return c.registry.Close()
}

type grpcPeerGetter struct {
	client pb.CacheServiceClient
	group  string
}

func (g *grpcPeerGetter) Get(ctx context.Context, in *pb.GetRequest, out *pb.GetResponse) ([]byte, error) {
	in.Group = g.group
	resp, err := g.client.Get(ctx, in)
	if err != nil {
		return nil, err
	}
	return resp.Value, nil
}
