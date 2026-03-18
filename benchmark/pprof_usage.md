# pprof 使用指南

## 简介

本项目已集成了 pprof 性能分析工具，可用于查看内存使用、CPU 占用、锁竞争等性能数据。

## 如何使用

### 1. 启动服务

首先启动带有 pprof 支持的服务器：

```bash
go run bench.go -mode=server -addr=localhost:8001
```

启动后会看到类似输出：
```
启动 pprof 服务器: 127.0.0.1:6060
启动服务: localhost:8001
```

### 2. 运行压测

在另一个终端运行压测：

```bash
go run bench.go -mode=bench -peers=localhost:8001 -concurrency=50 -requests=10000
```

### 3. 访问 pprof 界面

在浏览器中访问：
- http://127.0.0.1:6060/debug/pprof/ - pprof 主界面

### 4. 查看性能数据

#### 内存使用
- 访问：http://127.0.0.1:6060/debug/pprof/heap
- 或使用命令行：
  ```bash
  go tool pprof http://127.0.0.1:6060/debug/pprof/heap
  ```

#### CPU 使用
- 命令行：
  ```bash
  go tool pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=30
  ```

#### 锁竞争
- 命令行：
  ```bash
  go tool pprof http://127.0.0.1:6060/debug/pprof/mutex
  ```

#### 阻塞操作
- 命令行：
  ```bash
  go tool pprof http://127.0.0.1:6060/debug/pprof/block
  ```

## 常用 pprof 命令

进入 pprof 交互式界面后，可以使用以下命令：

- `top` - 查看占用最多的函数
- `top10` - 查看占用最多的前10个函数
- `list <函数名>` - 查看指定函数的详细信息
- `web` - 生成 SVG 格式的调用图
- `png` - 生成 PNG 格式的调用图
- `pdf` - 生成 PDF 格式的调用图

## 示例

### 查看内存使用

```bash
# 采集内存使用数据
go tool pprof http://127.0.0.1:6060/debug/pprof/heap

# 在交互式界面中查看
top
list distributed.NewGroup
web  # 会在浏览器中打开调用图
```

### 查看 CPU 使用

```bash
# 采集 30 秒的 CPU 使用数据
go tool pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=30

# 在交互式界面中查看
top
list testGet
web
```

### 查看锁竞争

```bash
# 采集锁竞争数据
go tool pprof http://127.0.0.1:6060/debug/pprof/mutex

# 在交互式界面中查看
top
list distributed
web
```
