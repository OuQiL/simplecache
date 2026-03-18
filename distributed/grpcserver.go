package distributed

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	pb "github.com/OuQiL/simplecache/cachepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

type GrpcServer struct {
	pb.UnimplementedCacheServiceServer
	group    *Group
	nodeID   string
	registry *EtcdRegister
}

type GrpcServerConfig struct {
	NodeID          string
	GroupName       string
	GroupCacheBytes int64
	GroupTimeout    time.Duration
	Getter          Getter
	Addr            string
	EtcdEndpoints   []string
}

func DefaultGrpcServerConfig(groupname string) GrpcServerConfig {
	return GrpcServerConfig{
		GroupName:       groupname,
		GroupCacheBytes: 1024 * 1024 * 1024,
		GroupTimeout:    100 * time.Millisecond,
	}
}

func NewGrpcServer(cfg GrpcServerConfig) (*GrpcServer, error) {
	// 创建 etcd 注册器
	reg, err := NewEtcdRegister(cfg.EtcdEndpoints, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry: %w", err)
	}

	group := NewGroup(
		cfg.GroupName,
		cfg.GroupCacheBytes,
		cfg.GroupTimeout,
		cfg.Getter,
	)

	return &GrpcServer{
		group:    group,
		nodeID:   cfg.NodeID,
		registry: reg,
	}, nil
}

// RegisterPeers 注册分布式节点
func (s *GrpcServer) RegisterPeers(peers PeerPicker) {
	s.group.RegisterPeers(peers)
}

// Get 获取缓存
func (s *GrpcServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	view, err := s.group.Get(ctx, req.Key)
	if err != nil {
		return &pb.GetResponse{Found: false}, nil
	}

	return &pb.GetResponse{
		Value: view.ByteSlice(),
		Found: true,
	}, nil
}

// Set 设置缓存
func (s *GrpcServer) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
	var ttl time.Duration
	if req.TtlSeconds > 0 {
		ttl = time.Duration(req.TtlSeconds) * time.Second
	}

	err := s.group.SetWithTTL(ctx, req.Key, req.Value, ttl)
	if err != nil {
		return &pb.SetResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &pb.SetResponse{Success: true}, nil
}

// Delete 删除缓存
func (s *GrpcServer) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	s.group.mainCache.Delete(req.Key)
	return &pb.DeleteResponse{Success: true}, nil
}

// HealthCheck 健康检查
func (s *GrpcServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	stats := s.group.mainCache.Stats()
	return &pb.HealthCheckResponse{
		Status:       pb.HealthCheckResponse_SERVING,
		EntriesCount: int64(stats.TotalEntries),
	}, nil
}

// Start 启动服务
func (s *GrpcServer) Start(addr string) error {
	kaParams := keepalive.ServerParameters{
		MaxConnectionIdle: 5 * time.Minute,
		Time:              10 * time.Second,
		Timeout:           1 * time.Second,
	}

	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(kaParams),
		grpc.MaxRecvMsgSize(10*1024*1024),
	)

	pb.RegisterCacheServiceServer(grpcServer, s)

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// 注册到 etcd
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := s.registry.Register(ctx, &NodeInfo{
		ID:        s.nodeID,
		nodeAddr:  addr,
		group:     s.group.Name(),
		startTime: time.Now(),
	}); err != nil {
		cancel()
		return fmt.Errorf("failed to register service: %w", err)
	}
	cancel()

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	log.Printf("GrpcServer started on %s, group: %s", addr, s.group.Name())
	return grpcServer.Serve(lis)
}

// Stop 停止服务
func (s *GrpcServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.registry.Deregister(ctx); err != nil {
		log.Printf("Failed to deregister: %v", err)
	}

	s.group.mainCache.Close()
	return s.registry.Close()
}
