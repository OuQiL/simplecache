package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/OuQiL/simplecache/distributed"
)

var (
	addr        = flag.String("addr", "localhost:8001", "节点地址")
	peers       = flag.String("peers", "", "其他节点地址，逗号分隔")
	testMode    = flag.String("mode", "server", "模式: server 或 bench")
	concurrency = flag.Int("concurrency", 50, "并发数")
	requests    = flag.Int("requests", 10000, "总请求数")
	groupName   = flag.String("group", "test", "缓存组名称")
)

func main() {
	flag.Parse()

	switch *testMode {
	case "server":
		startServer()
	case "bench":
		runBenchmark()
	default:
		fmt.Println("未知模式，使用 'server' 或 'bench'")
	}
}

func startServer() {
	// 初始化缓存组
	distributed.NewGroup(*groupName, 1024*1024*10, distributed.GetterFunc(func(key string) ([]byte, error) {
		// 模拟回源延迟
		time.Sleep(10 * time.Millisecond)
		return []byte(fmt.Sprintf("value for %s", key)), nil
	}))

	// 启动 HTTP 服务
	httpPool := distributed.NewHTTPPool(*addr)

	// 添加其他节点
	if *peers != "" {
		peerList := splitPeers(*peers)
		httpPool.Set(peerList...)
	}

	http.Handle("/", httpPool)

	fmt.Printf("启动服务: %s\n", *addr)
	if *peers != "" {
		fmt.Printf("其他节点: %v\n", splitPeers(*peers))
	}

	// 启动服务器
	server := &http.Server{
		Addr:    *addr,
		Handler: httpPool,
	}

	// 优雅关闭
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("服务启动失败: %v\n", err)
		}
	}()

	// 等待信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("服务关闭失败: %v\n", err)
	}

	fmt.Println("服务已关闭")
}

func runBenchmark() {
	if *peers == "" {
		fmt.Println("错误: 必须指定其他节点地址")
		return
	}

	peerList := splitPeers(*peers)
	if len(peerList) == 0 {
		fmt.Println("错误: 至少需要一个节点")
		return
	}

	fmt.Printf("开始压测，并发数: %d, 请求数: %d\n", *concurrency, *requests)
	fmt.Printf("测试节点: %v\n", peerList)

	// 预热
	fmt.Println("预热中...")
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("warmup-%d", i)
		testGet(peerList[0], key)
	}

	// 开始测试
	var wg sync.WaitGroup
	var mu sync.Mutex

	totalRequests := 0
	successRequests := 0
	totalLatency := time.Duration(0)
	errors := 0

	startTime := time.Now()

	// 并发测试
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				mu.Lock()
				if totalRequests >= *requests {
					mu.Unlock()
					break
				}
				reqID := totalRequests
				totalRequests++
				mu.Unlock()

				// 随机选择节点
				node := peerList[reqID%len(peerList)]
				key := fmt.Sprintf("test-%d", reqID)

				// 测试 GET
				start := time.Now()
				_, err := testGet(node, key)
				latency := time.Since(start)

				mu.Lock()
				totalLatency += latency
				if err == nil {
					successRequests++
				} else {
					errors++
				}
				mu.Unlock()

				// 保守的速率控制
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	// 生成报告
	fmt.Println("\n=== 压测报告 ===")
	fmt.Printf("总请求数: %d\n", totalRequests)
	fmt.Printf("成功请求: %d (%.2f%%)\n", successRequests, float64(successRequests)/float64(totalRequests)*100)
	fmt.Printf("失败请求: %d (%.2f%%)\n", errors, float64(errors)/float64(totalRequests)*100)
	fmt.Printf("总耗时: %v\n", elapsed)
	fmt.Printf("QPS: %.2f\n", float64(totalRequests)/elapsed.Seconds())
	if successRequests > 0 {
		fmt.Printf("平均延迟: %v\n", totalLatency/time.Duration(successRequests))
	}

	fmt.Println("=== 测试完成 ===")
}

func testGet(node, key string) ([]byte, error) {
	url := fmt.Sprintf("http://%s/_simplecache/%s/%s", node, *groupName, key)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return nil, err
	}

	return buf[:n], nil
}

func splitPeers(peers string) []string {
	if peers == "" {
		return []string{}
	}

	var result []string
	for _, p := range split(peers, ",") {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func split(s, sep string) []string {
	var result []string
	var current string

	for i := 0; i < len(s); i++ {
		if s[i:i+len(sep)] == sep {
			if current != "" {
				result = append(result, current)
				current = ""
			}
			i += len(sep) - 1
		} else {
			current += string(s[i])
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}
