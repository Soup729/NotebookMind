# Enterprise PDF AI - NotebookLM 功能验证脚本

## 前置条件

1. 确保 PostgreSQL 数据库已启动并可连接
2. 确保 Milvus/Zilliz Cloud 已配置
3. 确保 OpenAI API Key 已配置
4. 安装 Go 1.24+

## 运行测试

```bash
# 1. 复制环境变量模板
cp .env.example .env
# 编辑 .env 填入实际配置

# 2. 运行所有测试
go test -v ./internal/service/... -run Notebook
go test -v ./internal/repository/... -run Notebook

# 3. 或使用验证脚本（需要先设置环境变量）
./scripts/test_notebook.sh
```

---

## 单元测试文件

创建测试文件：

### 1. Notebook Service 单元测试
```go
// internal/service/notebook_service_test.go
```

### 2. Notebook Repository 单元测试
```go
// internal/repository/notebook_repository_test.go
```

### 3. 集成测试
```go
// tests/integration/notebook_integration_test.go
```
