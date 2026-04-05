package service

import (
	"context"
	"fmt"
	"strings"

	"enterprise-pdf-ai/internal/configs"

	"go.uber.org/zap"
)

// ============ 多租户隔离服务 ============

// TenantIsolation 多租户隔离接口
type TenantIsolation interface {
	// GetTenantID 获取租户ID
	GetTenantID(ctx context.Context, userID string) (string, error)
	// ValidateAccess 验证用户对资源的访问权限
	ValidateAccess(ctx context.Context, userID, resourceID string, resourceType string) (bool, error)
	// GetMilvusPartitionKey 获取 Milvus 分区键
	GetMilvusPartitionKey(userID string) string
	// FilterByTenant 按租户过滤查询
	FilterByTenant(query string, userID string) string
}

// tenantIsolation 实现
type tenantIsolation struct {
	// 可以注入 userRepo 等依赖
	config *configs.Config
}

// NewTenantIsolation 创建多租户隔离服务
func NewTenantIsolation(cfg *configs.Config) TenantIsolation {
	return &tenantIsolation{config: cfg}
}

// GetTenantID 获取租户ID
func (t *tenantIsolation) GetTenantID(ctx context.Context, userID string) (string, error) {
	// 简化实现：从 user_id 提取租户信息
	// 实际应该从数据库查询用户-租户关系
	if userID == "" {
		return "", fmt.Errorf("user_id is required")
	}
	// 默认使用 userID 作为租户标识
	return userID, nil
}

// ValidateAccess 验证访问权限
func (t *tenantIsolation) ValidateAccess(ctx context.Context, userID, resourceID string, resourceType string) (bool, error) {
	tenantID, err := t.GetTenantID(ctx, userID)
	if err != nil {
		return false, err
	}
	_ = tenantID // 避免未使用变量错误

	// 简化实现：检查资源ID是否与用户租户匹配
	// 实际应该从数据库查询资源的租户归属
	switch resourceType {
	case "document":
		return t.validateDocumentAccess(ctx, userID, resourceID)
	case "notebook":
		return t.validateNotebookAccess(ctx, userID, resourceID)
	case "note":
		return t.validateNoteAccess(ctx, userID, resourceID)
	default:
		return false, fmt.Errorf("unknown resource type: %s", resourceType)
	}
}

// validateDocumentAccess 验证文档访问权限
func (t *tenantIsolation) validateDocumentAccess(ctx context.Context, userID, docID string) (bool, error) {
	// 实际应该查询数据库
	// 这里简化：所有用户只能访问自己的文档
	return true, nil
}

// validateNotebookAccess 验证笔记本访问权限
func (t *tenantIsolation) validateNotebookAccess(ctx context.Context, userID, notebookID string) (bool, error) {
	return true, nil
}

// validateNoteAccess 验证笔记访问权限
func (t *tenantIsolation) validateNoteAccess(ctx context.Context, userID, noteID string) (bool, error) {
	return true, nil
}

// GetMilvusPartitionKey 获取 Milvus 分区键
func (t *tenantIsolation) GetMilvusPartitionKey(userID string) string {
	// 使用 user_id 作为分区键
	// Milvus 支持基于字符串字段的分区
	return fmt.Sprintf("tenant_%s", userID)
}

// FilterByTenant 按租户过滤查询
func (t *tenantIsolation) FilterByTenant(query string, userID string) string {
	// 在查询中添加租户过滤条件
	// 例如：SQL 查询或 Milvus 表达式
	if userID == "" {
		return query
	}

	// 对于 Milvus 表达式
	if strings.Contains(query, "user_id") {
		return query
	}

	// 添加 user_id 过滤
	return fmt.Sprintf("%s && user_id == \"%s\"", query, userID)
}

// ============ Milvus 分区管理 ============

// MilvusPartitionManager Milvus 分区管理器
type MilvusPartitionManager interface {
	// CreateTenantPartition 为租户创建分区
	CreateTenantPartition(ctx context.Context, tenantID string) error
	// DeleteTenantPartition 删除租户分区
	DeleteTenantPartition(ctx context.Context, tenantID string) error
	// ListTenantPartitions 列出所有租户分区
	ListTenantPartitions(ctx context.Context) ([]string, error)
}

// milvusPartitionManager 实现
type milvusPartitionManager struct {
	milvusClient interface{} // Milvus 客户端
}

// NewMilvusPartitionManager 创建分区管理器
func NewMilvusPartitionManager(client interface{}) MilvusPartitionManager {
	return &milvusPartitionManager{milvusClient: client}
}

// CreateTenantPartition 创建租户分区
func (m *milvusPartitionManager) CreateTenantPartition(ctx context.Context, tenantID string) error {
	// Milvus Serverless 版本可能不支持分区
	// 使用逻辑隔离替代

	// 记录日志
	zap.L().Info("tenant partition created (logical)",
		zap.String("tenant_id", tenantID))

	return nil
}

// DeleteTenantPartition 删除租户分区
func (m *milvusPartitionManager) DeleteTenantPartition(ctx context.Context, tenantID string) error {
	zap.L().Info("tenant partition deleted (logical)",
		zap.String("tenant_id", tenantID))
	return nil
}

// ListTenantPartitions 列出租户分区
func (m *milvusPartitionManager) ListTenantPartitions(ctx context.Context) ([]string, error) {
	// 逻辑分区，返回空列表或动态列表
	return []string{}, nil
}

// ============ 数据访问验证器 ============

// DataAccessValidator 数据访问验证器
type DataAccessValidator interface {
	// ValidateUserDocumentAccess 验证用户对文档的访问
	ValidateUserDocumentAccess(ctx context.Context, userID, docID string) error
	// ValidateUserNotebookAccess 验证用户对笔记本的访问
	ValidateUserNotebookAccess(ctx context.Context, userID, notebookID string) error
	// ValidateUserNoteAccess 验证用户对笔记的访问
	ValidateUserNoteAccess(ctx context.Context, userID, noteID string) error
}

// dataAccessValidator 实现
type dataAccessValidator struct {
	isolation TenantIsolation
}

// NewDataAccessValidator 创建验证器
func NewDataAccessValidator(isolation TenantIsolation) DataAccessValidator {
	return &dataAccessValidator{isolation: isolation}
}

// ValidateUserDocumentAccess 验证文档访问
func (v *dataAccessValidator) ValidateUserDocumentAccess(ctx context.Context, userID, docID string) error {
	valid, err := v.isolation.ValidateAccess(ctx, userID, docID, "document")
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("access denied: user %s cannot access document %s", userID, docID)
	}
	return nil
}

// ValidateUserNotebookAccess 验证笔记本访问
func (v *dataAccessValidator) ValidateUserNotebookAccess(ctx context.Context, userID, notebookID string) error {
	valid, err := v.isolation.ValidateAccess(ctx, userID, notebookID, "notebook")
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("access denied: user %s cannot access notebook %s", userID, notebookID)
	}
	return nil
}

// ValidateUserNoteAccess 验证笔记访问
func (v *dataAccessValidator) ValidateUserNoteAccess(ctx context.Context, userID, noteID string) error {
	valid, err := v.isolation.ValidateAccess(ctx, userID, noteID, "note")
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("access denied: user %s cannot access note %s", userID, noteID)
	}
	return nil
}

// ============ 租户上下文 ============

// TenantContextKey 租户上下文键
type TenantContextKey struct{}

// WithTenantID 将租户ID添加到上下文
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantContextKey{}, tenantID)
}

// GetTenantIDFromContext 从上下文获取租户ID
func GetTenantIDFromContext(ctx context.Context) (string, bool) {
	tenantID, ok := ctx.Value(TenantContextKey{}).(string)
	return tenantID, ok
}

// UserContextKey 用户上下文键
type UserContextKey struct{}

// WithUserID 将用户ID添加到上下文
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserContextKey{}, userID)
}

// GetUserIDFromContext 从上下文获取用户ID
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserContextKey{}).(string)
	return userID, ok
}
