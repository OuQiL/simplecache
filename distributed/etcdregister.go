package distributed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdRegister struct {
	client     *clientv3.Client
	serviceKey string
	nodeAddr   string
	leaseID    clientv3.LeaseID
	ttl        int64
	stop       chan struct{}
}

type NodeInfo struct {
	ID        string
	nodeAddr  string
	group     string
	startTime time.Time
}

func NewEtcdRegister(endpoints []string, ttl int64) (*EtcdRegister, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, errors.New("etcd register create client failed")
	}

	return &EtcdRegister{
		client: client,
		ttl:    ttl,
		stop:   make(chan struct{}),
	}, nil
}

func (e *EtcdRegister) Register(ctx context.Context, node *NodeInfo) error {
	e.nodeAddr = node.nodeAddr

	// 申请租约
	resp, err := e.client.Grant(ctx, e.ttl)
	if err != nil {
		return errors.New("etcd register grant lease failed")
	}
	e.leaseID = resp.ID

	// 注册节点
	_, err = e.client.Put(ctx, fmt.Sprintf("%s/%s", e.serviceKey, node.ID), node.nodeAddr, clientv3.WithLease(e.leaseID))
	if err != nil {
		return errors.New("etcd register put failed")
	}

	// 保持租约
	go e.keepAlive(ctx)

	return nil
}

// TODO:验证
func (e *EtcdRegister) keepAlive(ctx context.Context) {
	// 保持租约
	keepAliveCh, err := e.client.KeepAlive(ctx, e.leaseID)
	if err != nil {
		return
	}

	for keepAliveResp := range keepAliveCh {
		if keepAliveResp == nil {
			break
		}
	}
}

// Deregister 注销服务
func (e *EtcdRegister) Deregister(ctx context.Context) error {
	close(e.stop)
	_, err := e.client.Delete(ctx, e.serviceKey)
	if err != nil {
		return fmt.Errorf("failed to deregister: %w", err)
	}
	return nil
}

// Close 关闭连接
func (e *EtcdRegister) Close() error {
	return e.client.Close()
}

// Discover 发现服务 group->nodeinfo
func (e *EtcdRegister) Discover(ctx context.Context, group string) ([]NodeInfo, error) {
	// 从etcd获取服务节点
	resp, err := e.client.Get(ctx, fmt.Sprintf("%s/%s", e.serviceKey, group))
	if err != nil {
		return nil, fmt.Errorf("failed to discover: %w", err)
	}

	var nodes []NodeInfo
	for _, kv := range resp.Kvs {
		parts := strings.Split(string(kv.Key), "/")
		nodes = append(nodes, NodeInfo{
			ID:        parts[4],
			nodeAddr:  string(kv.Value),
			group:     group,
			startTime: time.Unix(0, kv.CreateRevision),
		})
	}
	return nodes, nil
}

func (e *EtcdRegister) Watch(ctx context.Context, group string, onChange func(node NodeInfo, added bool)) error {
	watchChan := e.client.Watch(ctx, fmt.Sprintf("%s/%s", e.serviceKey, group))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp := <-watchChan:
			for _, event := range resp.Events {
				parts := strings.Split(string(event.Kv.Key), "/")
				if len(parts) < 5 {
					continue
				}
				node := NodeInfo{
					ID:        parts[4],
					nodeAddr:  string(event.Kv.Value),
					group:     group,
					startTime: time.Unix(0, event.Kv.CreateRevision),
				}
				switch event.Type {
				case clientv3.EventTypePut:
					if onChange != nil {
						onChange(node, true)
					}
				case clientv3.EventTypeDelete:
					if onChange != nil {
						onChange(node, false)
					}
				}
			}
		}
	}
}
