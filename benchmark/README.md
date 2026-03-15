# 多节点压测指南(AI制作)

## 准备工作

1. 确保 Go 1.21+ 已安装
2. 确保项目依赖已安装：`go mod tidy`

## 运行方式

#### 启动多个节点

在不同的终端中分别运行：

```bash
# 节点 1
go run benchmark/bench.go -addr localhost:8001 -peers localhost:8002,localhost:8003 -mode server

# 节点 2  
go run benchmark/bench.go -addr localhost:8002 -peers localhost:8001,localhost:8003 -mode server

# 节点 3
go run benchmark/bench.go -addr localhost:8003 -peers localhost:8001,localhost:8002 -mode server
```

#### 运行压测

在另一个终端中运行：

```bash
go run benchmark/bench.go -mode bench -peers localhost:8001,localhost:8002,localhost:8003 -concurrency 10 -requests 1000
```



## 压测参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| -addr | 节点地址 | localhost:8001 |
| -peers | 其他节点地址（逗号分隔） | "" |
| -mode | 模式（server/bench） | server |
| -concurrency | 并发数 | 50 |
| -requests | 总请求数 | 10000 |
| -group | 缓存组名称 | test |

## 预期结果

压测完成后会生成类似以下的报告：

```
=== 压测报告 ===
总请求数: 1000
成功请求: 1000 (100.00%)
失败请求: 0 (0.00%)
总耗时: 1.234s
QPS: 810.25
平均延迟: 12.34ms
```

## 注意事项

1. **保守压测**：脚本默认使用较低的并发和请求数，适合初步测试
2. **资源监控**：建议同时监控 CPU、内存使用情况
3. **网络隔离**：多节点测试时建议在同一局域网内
4. **清理进程**：测试完成后确保清理所有 go 进程

## 故障排查

- 如果节点启动失败，检查端口是否被占用
- 如果压测失败，检查节点是否正常运行
- 查看节点日志了解详细错误信息
