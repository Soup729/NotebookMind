#!/bin/bash
# Enterprise PDF AI - NotebookLM 功能验证脚本
# 使用方法: ./scripts/test_notebook.sh

set -e

# 配置
API_BASE="${API_BASE_URL:-http://localhost:8080}/api/v1"
TOKEN=""
USER_ID=""

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试计数器
PASSED=0
FAILED=0

log_pass() {
    echo -e "${GREEN}[PASS]${NC}: $1"
    ((PASSED++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC}: $1"
    ((FAILED++))
}

log_info() {
    echo -e "${YELLOW}[INFO]${NC}: $1"
}

# ============ 辅助函数 ============

# 等待服务启动
wait_for_service() {
    log_info "等待服务启动..."
    for i in {1..30}; do
        if curl -s "$API_BASE/ping" > /dev/null 2>&1; then
            log_info "服务已就绪"
            return 0
        fi
        sleep 1
    done
    log_fail "服务启动超时"
    exit 1
}

# 获取认证 Token
get_token() {
    log_info "获取认证 Token..."

    RESPONSE=$(curl -s -X POST "$API_BASE/auth/login" \
        -H "Content-Type: application/json" \
        -d '{"email":"test@example.com","password":"password123"}' \
        2>/dev/null || echo '{"error":"login failed"}')

    TOKEN=$(echo $RESPONSE | grep -o '"token":"[^"]*"' | cut -d'"' -f4 || echo "")

    if [ -z "$TOKEN" ]; then
        log_fail "获取 Token 失败，请检查用户凭证"
        exit 1
    fi

    log_pass "获取 Token 成功"
}

# 创建测试用户
create_test_user() {
    log_info "创建测试用户..."

    curl -s -X POST "$API_BASE/auth/register" \
        -H "Content-Type: application/json" \
        -d '{"email":"test@example.com","password":"password123","name":"Test User"}' \
        > /dev/null 2>&1 || true

    get_token
}

# ============ API 测试 ============

# 测试 1: 创建笔记本
test_create_notebook() {
    log_info "测试 1: 创建笔记本"

    RESPONSE=$(curl -s -X POST "$API_BASE/notebooks" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d '{"title":"测试笔记本","description":"用于验证 NotebookLM 功能"}')

    NOTEBOOK_ID=$(echo $RESPONSE | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

    if [ -n "$NOTEBOOK_ID" ]; then
        log_pass "创建笔记本成功 (ID: $NOTEBOOK_ID)"
        echo "$NOTEBOOK_ID" > /tmp/notebook_id.txt
    else
        log_fail "创建笔记本失败"
        echo "$RESPONSE"
    fi
}

# 测试 2: 获取笔记本
test_get_notebook() {
    log_info "测试 2: 获取笔记本"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ]; then
        log_fail "笔记本 ID 不存在，请先运行测试 1"
        return
    fi

    RESPONSE=$(curl -s -X GET "$API_BASE/notebooks/$NOTEBOOK_ID" \
        -H "Authorization: Bearer $TOKEN")

    if echo "$RESPONSE" | grep -q '"title":"测试笔记本"'; then
        log_pass "获取笔记本成功"
    else
        log_fail "获取笔记本失败"
    fi
}

# 测试 3: 列出笔记本
test_list_notebooks() {
    log_info "测试 3: 列出笔记本"

    RESPONSE=$(curl -s -X GET "$API_BASE/notebooks" \
        -H "Authorization: Bearer $TOKEN")

    if echo "$RESPONSE" | grep -q '"items"'; then
        log_pass "列出笔记本成功"
    else
        log_fail "列出笔记本失败"
    fi
}

# 测试 4: 更新笔记本
test_update_notebook() {
    log_info "测试 4: 更新笔记本"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ]; then
        log_fail "笔记本 ID 不存在"
        return
    fi

    RESPONSE=$(curl -s -X PUT "$API_BASE/notebooks/$NOTEBOOK_ID" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d '{"title":"更新后的笔记本","status":"archived"}')

    if echo "$RESPONSE" | grep -q '"title":"更新后的笔记本"'; then
        log_pass "更新笔记本成功"
    else
        log_fail "更新笔记本失败"
    fi
}

# 测试 5: 上传文档
test_upload_document() {
    log_info "测试 5: 上传文档"

    # 创建临时 PDF 文件
    echo "%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R>>endobj
xref
0 4
0000000000 65535 f
0000000009 00000 n
0000000052 00000 n
0000000101 00000 n
trailer<</Size 4/Root 1 0 R>>
startxref
178
%%EOF" > /tmp/test.pdf

    RESPONSE=$(curl -s -X POST "$API_BASE/documents" \
        -H "Authorization: Bearer $TOKEN" \
        -F "file=@/tmp/test.pdf")

    DOCUMENT_ID=$(echo $RESPONSE | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

    if [ -n "$DOCUMENT_ID" ]; then
        log_pass "上传文档成功 (ID: $DOCUMENT_ID)"
        echo "$DOCUMENT_ID" > /tmp/document_id.txt
    else
        log_fail "上传文档失败"
        echo "$RESPONSE"
    fi
}

# 测试 6: 添加文档到笔记本
test_add_document_to_notebook() {
    log_info "测试 6: 添加文档到笔记本"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")
    DOCUMENT_ID=$(cat /tmp/document_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ] || [ -z "$DOCUMENT_ID" ]; then
        log_fail "笔记本或文档 ID 不存在"
        return
    fi

    RESPONSE=$(curl -s -X POST "$API_BASE/notebooks/$NOTEBOOK_ID/documents" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d "{\"document_id\":\"$DOCUMENT_ID\"}")

    if echo "$RESPONSE" | grep -q '"message":"document added to notebook"'; then
        log_pass "添加文档到笔记本成功"
    else
        log_fail "添加文档到笔记本失败"
    fi
}

# 测试 7: 列出笔记本中的文档
test_list_notebook_documents() {
    log_info "测试 7: 列出笔记本中的文档"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ]; then
        log_fail "笔记本 ID 不存在"
        return
    fi

    RESPONSE=$(curl -s -X GET "$API_BASE/notebooks/$NOTEBOOK_ID/documents" \
        -H "Authorization: Bearer $TOKEN")

    if echo "$RESPONSE" | grep -q '"items"'; then
        log_pass "列出笔记本文档成功"
    else
        log_fail "列出笔记本文档失败"
    fi
}

# 测试 8: 创建聊天会话
test_create_chat_session() {
    log_info "测试 8: 创建聊天会话"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ]; then
        log_fail "笔记本 ID 不存在"
        return
    fi

    RESPONSE=$(curl -s -X POST "$API_BASE/notebooks/$NOTEBOOK_ID/sessions" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d '{"title":"关于测试文档的讨论"}')

    SESSION_ID=$(echo $RESPONSE | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

    if [ -n "$SESSION_ID" ]; then
        log_pass "创建聊天会话成功 (ID: $SESSION_ID)"
        echo "$SESSION_ID" > /tmp/session_id.txt
    else
        log_fail "创建聊天会话失败"
    fi
}

# 测试 9: 列出聊天会话
test_list_chat_sessions() {
    log_info "测试 9: 列出聊天会话"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ]; then
        log_fail "笔记本 ID 不存在"
        return
    fi

    RESPONSE=$(curl -s -X GET "$API_BASE/notebooks/$NOTEBOOK_ID/sessions" \
        -H "Authorization: Bearer $TOKEN")

    if echo "$RESPONSE" | grep -q '"items"'; then
        log_pass "列出聊天会话成功"
    else
        log_fail "列出聊天会话失败"
    fi
}

# 测试 10: 流式问答
test_streaming_chat() {
    log_info "测试 10: 流式问答 (SSE)"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")
    SESSION_ID=$(cat /tmp/session_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ] || [ -z "$SESSION_ID" ]; then
        log_fail "笔记本或会话 ID 不存在"
        return
    fi

    # 使用 curl 进行 SSE 流式请求
    RESPONSE=$(curl -s -N -X POST "$API_BASE/notebooks/$NOTEBOOK_ID/sessions/$SESSION_ID/chat" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d '{"question":"这份文档的主要内容是什么？"}' 2>&1 | head -20 || echo "")

    if echo "$RESPONSE" | grep -qE '(data:|sources|content)'; then
        log_pass "流式问答成功收到响应"
    else
        log_fail "流式问答失败或无响应"
        echo "响应: $RESPONSE"
    fi
}

# 测试 11: 笔记本内搜索
test_notebook_search() {
    log_info "测试 11: 笔记本内向量搜索"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ]; then
        log_fail "笔记本 ID 不存在"
        return
    fi

    RESPONSE=$(curl -s -X POST "$API_BASE/notebooks/$NOTEBOOK_ID/search" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d '{"query":"测试内容","top_k":5}')

    if echo "$RESPONSE" | grep -q '"items"'; then
        log_pass "笔记本搜索成功"
    else
        log_fail "笔记本搜索失败"
    fi
}

# 测试 12: 删除文档
test_delete_document() {
    log_info "测试 12: 从笔记本移除文档"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")
    DOCUMENT_ID=$(cat /tmp/document_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ] || [ -z "$DOCUMENT_ID" ]; then
        log_fail "笔记本或文档 ID 不存在"
        return
    fi

    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
        "$API_BASE/notebooks/$NOTEBOOK_ID/documents/$DOCUMENT_ID" \
        -H "Authorization: Bearer $TOKEN")

    if [ "$RESPONSE" = "204" ]; then
        log_pass "从笔记本移除文档成功"
    else
        log_fail "从笔记本移除文档失败 (HTTP: $RESPONSE)"
    fi
}

# 测试 13: 删除笔记本
test_delete_notebook() {
    log_info "测试 13: 删除笔记本"

    NOTEBOOK_ID=$(cat /tmp/notebook_id.txt 2>/dev/null || echo "")

    if [ -z "$NOTEBOOK_ID" ]; then
        log_fail "笔记本 ID 不存在"
        return
    fi

    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
        "$API_BASE/notebooks/$NOTEBOOK_ID" \
        -H "Authorization: Bearer $TOKEN")

    if [ "$RESPONSE" = "204" ]; then
        log_pass "删除笔记本成功"
    else
        log_fail "删除笔记本失败 (HTTP: $RESPONSE)"
    fi
}

# ============ 健康检查测试 ============

test_health_check() {
    log_info "健康检查: API 服务"

    RESPONSE=$(curl -s "$API_BASE/ping")

    if echo "$RESPONSE" | grep -q '"status":"ok"'; then
        log_pass "API 服务健康"
    else
        log_fail "API 服务不健康"
    fi
}

# ============ 主函数 ============

main() {
    echo "=========================================="
    echo "  Enterprise PDF AI - NotebookLM 验证测试"
    echo "=========================================="
    echo ""

    # 等待服务
    wait_for_service

    # 创建测试用户并获取 Token
    create_test_user

    echo ""
    echo "--- 基础功能测试 ---"
    test_health_check
    test_create_notebook
    test_get_notebook
    test_list_notebooks
    test_update_notebook

    echo ""
    echo "--- 文档管理测试 ---"
    test_upload_document
    test_add_document_to_notebook
    test_list_notebook_documents

    echo ""
    echo "--- 问答功能测试 ---"
    test_create_chat_session
    test_list_chat_sessions
    test_streaming_chat
    test_notebook_search

    echo ""
    echo "--- 清理测试 ---"
    test_delete_document
    test_delete_notebook

    echo ""
    echo "=========================================="
    echo "  测试结果汇总"
    echo "=========================================="
    echo -e "[PASS] 通过: $PASSED"
    echo -e "[FAIL] 失败: $FAILED"
    echo ""

    # 清理临时文件
    rm -f /tmp/notebook_id.txt /tmp/document_id.txt /tmp/session_id.txt /tmp/test.pdf

    if [ $FAILED -eq 0 ]; then
        echo -e "${GREEN}所有测试通过!${NC}"
        exit 0
    else
        echo -e "${RED}部分测试失败，请检查上述输出${NC}"
        exit 1
    fi
}

# 运行主函数
main "$@"
