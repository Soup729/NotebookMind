# Enterprise PDF AI - NotebookLM 功能验证脚本 (PowerShell)
# 使用方法: .\scripts\test_notebook.ps1

param(
    [string]$ApiBase = "http://localhost:8080/api/v1"
)

# 配置
$TOKEN = ""
$USER_ID = ""

# 测试计数器
$PASSED = 0
$FAILED = 0

# 辅助函数
function Log-Pass {
    param([string]$Message)
    Write-Host "[PASS] $Message" -ForegroundColor Green
    $script:PASSED++
}

function Log-Fail {
    param([string]$Message)
    Write-Host "[FAIL] $Message" -ForegroundColor Red
    $script:FAILED++
}

function Log-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Yellow
}

# 等待服务启动
function Wait-ForService {
    Log-Info "等待服务启动..."
    $maxAttempts = 30
    for ($i = 0; $i -lt $maxAttempts; $i++) {
        try {
            $response = Invoke-RestMethod -Uri "$ApiBase/ping" -Method GET -TimeoutSec 2 -ErrorAction SilentlyContinue
            if ($response.status -eq "ok") {
                Log-Info "服务已就绪"
                return $true
            }
        } catch {}
        Start-Sleep -Seconds 1
    }
    Log-Fail "服务启动超时"
    return $false
}

# 获取认证 Token
function Get-Token {
    Log-Info "获取认证 Token..."

    try {
        $body = @{
            email    = "test@example.com"
            password = "password123"
        } | ConvertTo-Json

        $response = Invoke-RestMethod -Uri "$ApiBase/auth/login" -Method POST -Body $body -ContentType "application/json" -TimeoutSec 10

        if ($response.token) {
            $script:TOKEN = $response.token
            Log-Pass "获取 Token 成功"
            return $true
        }
    } catch {
        Log-Info "登录失败，尝试注册..."
    }

    return $false
}

# 创建测试用户
function New-TestUser {
    Log-Info "创建测试用户..."

    try {
        $body = @{
            email    = "test@example.com"
            password = "password123"
            name     = "Test User"
        } | ConvertTo-Json

        $null = Invoke-RestMethod -Uri "$ApiBase/auth/register" -Method POST -Body $body -ContentType "application/json" -TimeoutSec 10 -ErrorAction SilentlyContinue
    } catch {}

    return Get-Token
}

# ============ API 测试函数 ============

# 测试 1: 创建笔记本
function Test-CreateNotebook {
    Log-Info "测试 1: 创建笔记本"

    $body = @{
        title       = "测试笔记本"
        description = "用于验证 NotebookLM 功能"
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks" -Method POST -Body $body -ContentType "application/json" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.notebook.id) {
            $script:NOTEBOOK_ID = $response.notebook.id
            Log-Pass "创建笔记本成功 (ID: $NOTEBOOK_ID)"
            $script:NOTEBOOK_ID | Out-File -FilePath "$env:TEMP\notebook_id.txt" -Encoding UTF8
            return $true
        }
    } catch {
        Log-Fail "创建笔记本失败: $_"
    }
    return $false
}

# 测试 2: 获取笔记本
function Test-GetNotebook {
    Log-Info "测试 2: 获取笔记本"

    if (-not $script:NOTEBOOK_ID) {
        Log-Fail "笔记本 ID 不存在，请先运行测试 1"
        return $false
    }

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks/$NOTEBOOK_ID" -Method GET -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.notebook.title -eq "测试笔记本") {
            Log-Pass "获取笔记本成功"
            return $true
        }
    } catch {
        Log-Fail "获取笔记本失败: $_"
    }
    return $false
}

# 测试 3: 列出笔记本
function Test-ListNotebooks {
    Log-Info "测试 3: 列出笔记本"

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks" -Method GET -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.items) {
            Log-Pass "列出笔记本成功"
            return $true
        }
    } catch {
        Log-Fail "列出笔记本失败: $_"
    }
    return $false
}

# 测试 4: 更新笔记本
function Test-UpdateNotebook {
    Log-Info "测试 4: 更新笔记本"

    if (-not $script:NOTEBOOK_ID) {
        Log-Fail "笔记本 ID 不存在"
        return $false
    }

    $body = @{
        title  = "更新后的笔记本"
        status = "archived"
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks/$NOTEBOOK_ID" -Method PUT -Body $body -ContentType "application/json" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.notebook.title -eq "更新后的笔记本") {
            Log-Pass "更新笔记本成功"
            return $true
        }
    } catch {
        Log-Fail "更新笔记本失败: $_"
    }
    return $false
}

# 测试 5: 上传文档
function Test-UploadDocument {
    Log-Info "测试 5: 上传文档"

    # 创建临时 PDF 文件
    $pdfContent = "%PDF-1.4
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
%%EOF"

    $tempPdf = "$env:TEMP\test_$([guid]::NewGuid().ToString('N')).pdf"
    $pdfContent | Out-File -FilePath $tempPdf -Encoding UTF8

    try {
        $fileBytes = [System.IO.File]::ReadAllBytes($tempPdf)
        $fileEnc = [System.Text.Encoding]::GetEncoding("ISO-8859-1")
        $encodedContent = $fileEnc.GetString($fileBytes)

        $boundary = [System.Guid]::NewGuid().ToString()
        $body = "--$boundary`r`n"
        $body += "Content-Disposition: form-data; name=`"file`"; filename=`"test.pdf`"`r`n"
        $body += "Content-Type: application/pdf`r`n`r`n"
        $body += $encodedContent
        $body += "`r`n--$boundary--`r`n"

        $response = Invoke-RestMethod -Uri "$ApiBase/documents" -Method POST -Body $body -ContentType "multipart/form-data; boundary=$boundary" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 30

        if ($response.id) {
            $script:DOCUMENT_ID = $response.id
            Log-Pass "上传文档成功 (ID: $DOCUMENT_ID)"
            $script:DOCUMENT_ID | Out-File -FilePath "$env:TEMP\document_id.txt" -Encoding UTF8
            return $true
        }
    } catch {
        Log-Fail "上传文档失败: $_"
    } finally {
        Remove-Item $tempPdf -ErrorAction SilentlyContinue
    }
    return $false
}

# 测试 6: 添加文档到笔记本
function Test-AddDocumentToNotebook {
    Log-Info "测试 6: 添加文档到笔记本"

    if (-not $script:NOTEBOOK_ID -or -not $script:DOCUMENT_ID) {
        Log-Fail "笔记本或文档 ID 不存在"
        return $false
    }

    $body = @{
        document_id = $script:DOCUMENT_ID
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks/$NOTEBOOK_ID/documents" -Method POST -Body $body -ContentType "application/json" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.message -eq "document added to notebook") {
            Log-Pass "添加文档到笔记本成功"
            return $true
        }
    } catch {
        Log-Fail "添加文档到笔记本失败: $_"
    }
    return $false
}

# 测试 7: 列出笔记本中的文档
function Test-ListNotebookDocuments {
    Log-Info "测试 7: 列出笔记本中的文档"

    if (-not $script:NOTEBOOK_ID) {
        Log-Fail "笔记本 ID 不存在"
        return $false
    }

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks/$NOTEBOOK_ID/documents" -Method GET -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.items) {
            Log-Pass "列出笔记本文档成功"
            return $true
        }
    } catch {
        Log-Fail "列出笔记本文档失败: $_"
    }
    return $false
}

# 测试 8: 创建聊天会话
function Test-CreateChatSession {
    Log-Info "测试 8: 创建聊天会话"

    if (-not $script:NOTEBOOK_ID) {
        Log-Fail "笔记本 ID 不存在"
        return $false
    }

    $body = @{
        title = "关于测试文档的讨论"
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks/$NOTEBOOK_ID/sessions" -Method POST -Body $body -ContentType "application/json" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.session.id) {
            $script:SESSION_ID = $response.session.id
            Log-Pass "创建聊天会话成功 (ID: $SESSION_ID)"
            $script:SESSION_ID | Out-File -FilePath "$env:TEMP\session_id.txt" -Encoding UTF8
            return $true
        }
    } catch {
        Log-Fail "创建聊天会话失败: $_"
    }
    return $false
}

# 测试 9: 列出聊天会话
function Test-ListChatSessions {
    Log-Info "测试 9: 列出聊天会话"

    if (-not $script:NOTEBOOK_ID) {
        Log-Fail "笔记本 ID 不存在"
        return $false
    }

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks/$NOTEBOOK_ID/sessions" -Method GET -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.items) {
            Log-Pass "列出聊天会话成功"
            return $true
        }
    } catch {
        Log-Fail "列出聊天会话失败: $_"
    }
    return $false
}

# 测试 10: 流式问答
function Test-StreamingChat {
    Log-Info "测试 10: 流式问答 (SSE)"

    if (-not $script:NOTEBOOK_ID -or -not $script:SESSION_ID) {
        Log-Fail "笔记本或会话 ID 不存在"
        return $false
    }

    $body = @{
        question = "这份文档的主要内容是什么？"
    } | ConvertTo-Json

    try {
        # 使用 WebClient 进行 SSE 请求
        $wc = New-Object System.Net.WebClient
        $wc.Headers["Authorization"] = "Bearer $TOKEN"
        $wc.Headers["Content-Type"] = "application/json"

        $response = $wc.UploadString("$ApiBase/notebooks/$NOTEBOOK_ID/sessions/$SESSION_ID/chat", "POST", $body)
        $wc.Dispose()

        if ($response -match 'data:|sources|content') {
            Log-Pass "流式问答成功收到响应"
            return $true
        }
    } catch {
        Log-Fail "流式问答失败: $_"
    }
    return $false
}

# 测试 11: 笔记本内搜索
function Test-NotebookSearch {
    Log-Info "测试 11: 笔记本内向量搜索"

    if (-not $script:NOTEBOOK_ID) {
        Log-Fail "笔记本 ID 不存在"
        return $false
    }

    $body = @{
        query = "测试内容"
        top_k = 5
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notebooks/$NOTEBOOK_ID/search" -Method POST -Body $body -ContentType "application/json" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.items) {
            Log-Pass "笔记本搜索成功"
            return $true
        }
    } catch {
        Log-Fail "笔记本搜索失败: $_"
    }
    return $false
}

# 测试 12: 删除文档
function Test-DeleteDocument {
    Log-Info "测试 12: 从笔记本移除文档"

    if (-not $script:NOTEBOOK_ID -or -not $script:DOCUMENT_ID) {
        Log-Fail "笔记本或文档 ID 不存在"
        return $false
    }

    try {
        $statusCode = (Invoke-WebRequest -Uri "$ApiBase/notebooks/$NOTEBOOK_ID/documents/$DOCUMENT_ID" -Method DELETE -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10 -ErrorAction SilentlyContinue).StatusCode

        if ($statusCode -eq 204) {
            Log-Pass "从笔记本移除文档成功"
            return $true
        }
    } catch {
        if ($_.Exception.Response.StatusCode -eq 204) {
            Log-Pass "从笔记本移除文档成功"
            return $true
        }
    }
    Log-Fail "从笔记本移除文档失败"
    return $false
}

# 测试 13: 删除笔记本
function Test-DeleteNotebook {
    Log-Info "测试 13: 删除笔记本"

    if (-not $script:NOTEBOOK_ID) {
        Log-Fail "笔记本 ID 不存在"
        return $false
    }

    try {
        $statusCode = (Invoke-WebRequest -Uri "$ApiBase/notebooks/$NOTEBOOK_ID" -Method DELETE -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10 -ErrorAction SilentlyContinue).StatusCode

        if ($statusCode -eq 204) {
            Log-Pass "删除笔记本成功"
            return $true
        }
    } catch {
        if ($_.Exception.Response.StatusCode -eq 204) {
            Log-Pass "删除笔记本成功"
            return $true
        }
    }
    Log-Fail "删除笔记本失败"
    return $false
}

# 健康检查
function Test-HealthCheck {
    Log-Info "健康检查: API 服务"

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/ping" -Method GET -TimeoutSec 10

        if ($response.status -eq "ok") {
            Log-Pass "API 服务健康"
            return $true
        }
    } catch {
        Log-Fail "API 服务不健康: $_"
    }
    return $false
}

# ============ 笔记测试函数 ============

# 测试 14: 创建笔记
function Test-CreateNote {
    Log-Info "测试 14: 创建笔记"

    $body = @{
        notebook_id = $script:NOTEBOOK_ID
        title      = "研究笔记测试"
        content    = "这是一条测试笔记内容，用于验证笔记功能"
        type       = "custom"
        is_pinned  = $false
        tags       = @("测试", "重要")
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notes" -Method POST -Body $body -ContentType "application/json" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.note.id) {
            $script:NOTE_ID = $response.note.id
            Log-Pass "创建笔记成功 (ID: $NOTE_ID)"
            $script:NOTE_ID | Out-File -FilePath "$env:TEMP\note_id.txt" -Encoding UTF8
            return $true
        }
    } catch {
        Log-Fail "创建笔记失败: $_"
    }
    return $false
}

# 测试 15: 获取笔记
function Test-GetNote {
    Log-Info "测试 15: 获取笔记"

    if (-not $script:NOTE_ID) {
        Log-Fail "笔记 ID 不存在，请先运行测试 14"
        return $false
    }

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notes/$NOTE_ID" -Method GET -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.note.title -eq "研究笔记测试") {
            Log-Pass "获取笔记成功"
            return $true
        }
    } catch {
        Log-Fail "获取笔记失败: $_"
    }
    return $false
}

# 测试 16: 列出笔记
function Test-ListNotes {
    Log-Info "测试 16: 列出笔记"

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notes" -Method GET -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.items) {
            Log-Pass "列出笔记成功"
            return $true
        }
    } catch {
        Log-Fail "列出笔记失败: $_"
    }
    return $false
}

# 测试 17: 更新笔记
function Test-UpdateNote {
    Log-Info "测试 17: 更新笔记"

    if (-not $script:NOTE_ID) {
        Log-Fail "笔记 ID 不存在"
        return $false
    }

    $body = @{
        title     = "更新后的笔记标题"
        is_pinned = $true
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notes/$NOTE_ID" -Method PUT -Body $body -ContentType "application/json" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.note.title -eq "更新后的笔记标题") {
            Log-Pass "更新笔记成功"
            return $true
        }
    } catch {
        Log-Fail "更新笔记失败: $_"
    }
    return $false
}

# 测试 18: 钉住笔记
function Test-PinNote {
    Log-Info "测试 18: 钉住笔记"

    if (-not $script:NOTE_ID) {
        Log-Fail "笔记 ID 不存在"
        return $false
    }

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notes/$NOTE_ID/pin" -Method POST -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.message) {
            Log-Pass "钉住笔记成功"
            return $true
        }
    } catch {
        Log-Fail "钉住笔记失败: $_"
    }
    return $false
}

# 测试 19: 添加笔记标签
function Test-AddNoteTag {
    Log-Info "测试 19: 添加笔记标签"

    if (-not $script:NOTE_ID) {
        Log-Fail "笔记 ID 不存在"
        return $false
    }

    $body = @{
        tag = "新标签"
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notes/$NOTE_ID/tags" -Method POST -Body $body -ContentType "application/json" -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.message -eq "tag added") {
            Log-Pass "添加笔记标签成功"
            return $true
        }
    } catch {
        Log-Fail "添加笔记标签失败: $_"
    }
    return $false
}

# 测试 20: 按标签搜索笔记
function Test-SearchNotesByTag {
    Log-Info "测试 20: 按标签搜索笔记"

    try {
        $response = Invoke-RestMethod -Uri "$ApiBase/notes/tags/search?tag=测试" -Method GET -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10

        if ($response.items) {
            Log-Pass "按标签搜索笔记成功"
            return $true
        }
    } catch {
        Log-Fail "按标签搜索笔记失败: $_"
    }
    return $false
}

# 测试 21: 删除笔记
function Test-DeleteNote {
    Log-Info "测试 21: 删除笔记"

    if (-not $script:NOTE_ID) {
        Log-Fail "笔记 ID 不存在"
        return $false
    }

    try {
        $statusCode = (Invoke-WebRequest -Uri "$ApiBase/notes/$NOTE_ID" -Method DELETE -Headers @{ Authorization = "Bearer $TOKEN" } -TimeoutSec 10 -ErrorAction SilentlyContinue).StatusCode

        if ($statusCode -eq 200) {
            Log-Pass "删除笔记成功"
            return $true
        }
    } catch {
        if ($_.Exception.Response.StatusCode -eq 200) {
            Log-Pass "删除笔记成功"
            return $true
        }
    }
    Log-Fail "删除笔记失败"
    return $false
}

# ============ 主函数 ============

function Main {
    Write-Host "==========================================" -ForegroundColor Cyan
    Write-Host "  Enterprise PDF AI - NotebookLM 验证测试" -ForegroundColor Cyan
    Write-Host "==========================================" -ForegroundColor Cyan
    Write-Host ""

    # 全局变量
    $script:NOTEBOOK_ID = ""
    $script:DOCUMENT_ID = ""
    $script:SESSION_ID = ""
    $script:NOTE_ID = ""

    # 等待服务
    if (-not (Wait-ForService)) {
        exit 1
    }

    # 创建测试用户并获取 Token
    if (-not (New-TestUser)) {
        Log-Fail "无法获取认证 Token"
        exit 1
    }

    Write-Host ""
    Write-Host "--- 基础功能测试 ---" -ForegroundColor Cyan

    Test-HealthCheck
    Test-CreateNotebook
    Test-GetNotebook
    Test-ListNotebooks
    Test-UpdateNotebook

    Write-Host ""
    Write-Host "--- 文档管理测试 ---" -ForegroundColor Cyan

    Test-UploadDocument
    Test-AddDocumentToNotebook
    Test-ListNotebookDocuments

    Write-Host ""
    Write-Host "--- 问答功能测试 ---" -ForegroundColor Cyan

    Test-CreateChatSession
    Test-ListChatSessions
    Test-StreamingChat
    Test-NotebookSearch

    Write-Host ""
    Write-Host "--- 研究笔记测试 ---" -ForegroundColor Cyan

    Test-CreateNote
    Test-GetNote
    Test-ListNotes
    Test-UpdateNote
    Test-PinNote
    Test-AddNoteTag
    Test-SearchNotesByTag
    Test-DeleteNote

    Write-Host ""
    Write-Host "--- 清理测试 ---" -ForegroundColor Cyan

    Test-DeleteDocument
    Test-DeleteNotebook

    Write-Host ""
    Write-Host "==========================================" -ForegroundColor Cyan
    Write-Host "  测试结果汇总" -ForegroundColor Cyan
    Write-Host "==========================================" -ForegroundColor Cyan
    Write-Host "[PASS] 通过: $PASSED" -ForegroundColor Green
    Write-Host "[FAIL] 失败: $FAILED" -ForegroundColor Red
    Write-Host ""

    # 清理临时文件
    Remove-Item "$env:TEMP\notebook_id.txt" -ErrorAction SilentlyContinue
    Remove-Item "$env:TEMP\document_id.txt" -ErrorAction SilentlyContinue
    Remove-Item "$env:TEMP\session_id.txt" -ErrorAction SilentlyContinue
    Remove-Item "$env:TEMP\note_id.txt" -ErrorAction SilentlyContinue

    if ($FAILED -eq 0) {
        Write-Host "所有测试通过!" -ForegroundColor Green
        exit 0
    } else {
        Write-Host "部分测试失败，请检查上述输出" -ForegroundColor Red
        exit 1
    }
}

# 运行主函数
Main
