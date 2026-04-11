#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
NotebookMind - LLM-as-a-Judge 离线评估脚本

基于 LLM-as-a-Judge 思想，自动调用本地 API 进行问答，
并使用大模型裁判 (GPT-4o / Claude) 评估回答质量。

支持的评估维度：
  - Groundedness (忠实度): 回答是否基于提供的上下文，无幻觉
  - Relevance (相关性): 回答是否与问题相关
  - Completeness (完整性): 回答是否覆盖了问题的所有方面
  - Citation Precision (引用精确度): 引用来源是否准确
  - Hallucination Detection (幻觉检测): 检测是否有编造信息

输出指标：
  - Recall@K: 检索召回率（基于标注的期望文档）
  - Groundedness Score: 忠实度得分 [0-5]
  - Citation Precision: 引用准确率
  - Hallucination Rate: 幻觉率
  - Overall Score: 综合评分

用法:
    # 基础运行（需要先启动 API 服务）
    python scripts/notebook_eval.py --api-base http://localhost:8080/api/v1

    # 指定评测集和输出文件
    python scripts/notebook_eval.py \
        --dataset tests/data/eval_dataset.jsonl \
        --output eval_results_$(date +%Y%m%d).json

    # 使用特定 Judge 模型
    python scripts/notebook_eval.py \
        --judge-model gpt-4o \
        --judge-api-key sk-xxx \
        --judge-api-url https://api.openai.com/v1

环境变量:
    EVAL_API_BASE       - API 基础 URL (默认 http://localhost:8080/api/v1)
    EVAL_JUDGE_MODEL    - Judge 模型名称 (默认 gpt-4o-mini)
    EVAL_JUDGE_API_KEY  - Judge API Key
    EVAL_JUDGE_API_URL  - Judge API URL
"""

import argparse
import io
import json
import os
import sys
import time
import uuid
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Optional, List, Dict, Any
from datetime import datetime
import urllib.request
import urllib.error
import ssl


# ============================================================
# 加载项目 .env 配置（项目根目录）
# ============================================================
def load_dotenv(env_path: str = None) -> Dict[str, str]:
    """加载 .env 文件到环境变量（不覆盖已有值）"""
    if env_path is None:
        # 脚本在 scripts/ 目录，.env 在项目根目录
        env_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '.env')

    result = {}
    if os.path.isfile(env_path):
        with open(env_path, 'r', encoding='utf-8') as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line or line.startswith('#'):
                    continue
                if '=' not in line:
                    continue
                key, _, value = line.partition('=')
                key = key.strip()
                value = value.strip().strip('"').strip("'")
                # 只在环境变量不存在时才设置（命令行优先级更高）
                if key and key not in os.environ:
                    os.environ[key] = value
                    result[key] = value
    return result


# 启动时自动加载
_loaded_envs = load_dotenv()


@dataclass
class EvalItem:
    """评测集单条数据"""
    id: str
    category: str
    question: str
    expected_answer: str
    expected_sources: List[str]
    difficulty: str = "medium"
    tags: List[str] = field(default_factory=list)
    turns: Optional[List[Dict]] = None  # 多轮对话


@dataclass
class EvalResult:
    """单条评测结果"""
    item_id: str
    category: str
    question: str
    expected_answer: str
    actual_answer: str = ""
    sources: List[Dict] = field(default_factory=list)
    latency_ms: int = 0
    error: str = ""

    # Judge 评分
    groundedness_score: float = 0.0      # [0-5] 忠实度
    relevance_score: float = 0.0         # [0-5] 相关性
    completeness_score: float = 0.0      # [0-5] 完整性
    citation_precision: float = 0.0      # [0-1] 引用精确度
    has_hallucination: bool = False       # 是否有幻觉
    hallucination_details: str = ""      # 幻觉详情
    judge_reasoning: str = ""            # Judge 推理过程
    overall_score: float = 0.0           # 综合分 [0-100]

    # 检索指标
    recall_at_k: float = 0.0            # 检索召回率

    # Token 用量
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0


@dataclass
class EvalReport:
    """评测报告汇总"""
    run_id: str
    timestamp: str
    total_items: int = 0
    success_count: int = 0
    error_count: int = 0
    avg_latency_ms: float = 0.0
    p95_latency_ms: float = 0.0

    # 平均分数
    avg_groundedness: float = 0.0
    avg_relevance: float = 0.0
    avg_completeness: float = 0.0
    avg_citation_precision: float = 0.0
    avg_overall_score: float = 0.0

    # 关键指标
    hallucination_rate: float = 0.0     # 幻觉率
    avg_recall_at_k: float = 0.0        # 平均检索召回率

    # 分类别统计
    category_stats: Dict[str, Dict] = field(default_factory=dict)

    # 详细结果
    results: List[EvalResult] = field(default_factory=list)


def load_dataset(path: str) -> List[EvalItem]:
    """加载 JSONL 格式的评测集"""
    items = []
    with open(path, 'r', encoding='utf-8') as f:
        for line_num, line in enumerate(f, 1):
            line = line.strip()
            if not line or line.startswith('#'):
                continue
            try:
                data = json.loads(line)
                item = EvalItem(
                    id=data.get('id', f'item-{line_num}'),
                    category=data.get('category', 'unknown'),
                    question=data.get('question', ''),
                    expected_answer=data.get('expected_answer', ''),
                    expected_sources=data.get('expected_sources', []),
                    difficulty=data.get('difficulty', 'medium'),
                    tags=data.get('tags', []),
                    turns=data.get('turns'),
                )
                items.append(item)
            except json.JSONDecodeError as e:
                print(f"[WARN] 跳过第 {line_num} 行: JSON 解析错误 - {e}")
                continue
    return items


class APIClient:
    """NotebookMind API 客户端"""

    def __init__(self, base_url: str, token: str = ""):
        self.base_url = base_url.rstrip('/')
        self.token = token
        self.session_id: Optional[str] = None
        self.notebook_id: Optional[str] = None

    def _headers(self) -> Dict[str, str]:
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    def _request(self, method: str, path: str, data: Any = None, timeout: int = 120) -> Dict:
        url = f"{self.base_url}{path}"
        body = json.dumps(data).encode() if data else None
        req = urllib.request.Request(url, data=body, headers=self._headers(), method=method)

        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE

        try:
            with urllib.request.urlopen(req, timeout=timeout, context=ctx) as resp:
                return json.loads(resp.read().decode())
        except urllib.error.HTTPError as e:
            error_body = e.read().decode() if e.fp else ""
            raise Exception(f"HTTP {e.code}: {error_body}") from e
        except urllib.error.URLError as e:
            raise Exception(f"连接失败: {e.reason}") from e

    def login(self, username: str = "eval_user", password: str = "eval_pass_123") -> bool:
        """登录获取 token（如果用户不存在会自动注册）"""
        email = f"{username}@eval.local"
        try:
            # 尝试注册（后端要求: name, email, password）
            try:
                self._request("POST", "/auth/register", {
                    "name": username,
                    "email": email,
                    "password": password,
                })
            except Exception:
                pass  # 用户可能已存在

            # 登录（后端要求: email, password，不是 username）
            resp = self._request("POST", "/auth/login", {
                "email": email,
                "password": password
            })
            self.token = resp.get("token", resp.get("access_token", ""))
            return bool(self.token)
        except Exception as e:
            print(f"[WARN] 登录失败: {e}，将尝试无认证访问")
            return False

    def create_notebook(self, title: str = "Eval Notebook") -> str:
        """创建评测用笔记本"""
        resp = self._request("POST", "/notebooks", {"title": title})
        notebook = resp.get("notebook", resp)
        nid = notebook.get("ID") or notebook.get("id")
        if not nid:
            raise KeyError(f"notebook id not found in response: {list(notebook.keys())}")
        self.notebook_id = nid
        return self.notebook_id

    def get_notebooks(self) -> List[str]:
        """获取笔记本列表"""
        resp = self._request("GET", "/notebooks")
        notebooks = resp.get("items", [])
        return [(nb.get("ID") or nb.get("id")) for nb in notebooks]

    def upload_document(self, file_path: str, notebook_id: str) -> Dict:
        """上传 PDF 文档到指定笔记本"""
        import mimetypes

        url = f"{self.base_url}/documents"
        boundary = "----PythonEvalBoundary" + uuid.uuid4().hex[:16]

        filename = os.path.basename(file_path)
        with open(file_path, 'rb') as f:
            file_data = f.read()

        mime_type = mimetypes.guess_type(filename)[0] or 'application/pdf'

        # 用 BytesIO 构建 multipart body（避免 bytes/str 混淆）
        buf = io.BytesIO()

        # file field
        buf.write(f'--{boundary}\r\n'.encode())
        buf.write(f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'.encode())
        buf.write(f'Content-Type: {mime_type}\r\n'.encode())
        buf.write(b'\r\n')
        buf.write(file_data)
        buf.write(b'\r\n')

        # notebook_id field
        buf.write(f'--{boundary}\r\n'.encode())
        buf.write(b'Content-Disposition: form-data; name="notebook_id"\r\n')
        buf.write(b'\r\n')
        buf.write(notebook_id.encode('utf-8'))
        buf.write(b'\r\n')

        # end
        buf.write(f'--{boundary}--\r\n'.encode())

        body_data = buf.getvalue()

        req = urllib.request.Request(
            url,
            data=body_data,
            headers={
                "Content-Type": f"multipart/form-data; boundary={boundary}",
                **{k: v for k, v in self._headers().items() if k != "Content-Type"},
            },
            method="POST"
        )

        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE

        try:
            with urllib.request.urlopen(req, timeout=120, context=ctx) as resp:
                return json.loads(resp.read().decode())
        except urllib.error.HTTPError as e:
            error_body = e.read().decode() if e.fp else ""
            raise Exception(f"HTTP {e.code}: {error_body}") from e

    def list_documents(self) -> List[Dict]:
        """获取当前用户的文档列表"""
        resp = self._request("GET", "/documents")
        return resp.get("items", [])

    def wait_for_documents(self, timeout: int = 120) -> bool:
        """轮询等待所有文档处理完成，返回是否全部成功"""
        start = time.time()
        first_check = True
        while time.time() - start < timeout:
            docs = self.list_documents()
            if not docs:
                if first_check:
                    print(f"      文档列表为空，等待中...")
                    first_check = False
                time.sleep(2)
                continue
            statuses = [d.get("status") or d.get("Status", "unknown") for d in docs]
            completed = sum(1 for s in statuses if s == "completed")
            failed = sum(1 for s in statuses if s == "failed")
            processing = sum(1 for s in statuses if s == "processing")

            # 收集失败文档的详情
            failed_docs = []
            for d, s in zip(docs, statuses):
                if s == "failed":
                    fname = d.get("file_name") or d.get("FileName") or d.get("filename", "?")
                    did = d.get("ID") or d.get("id", "?")
                    failed_docs.append(f"  - {fname} ({did[:8]}...)")

            print(f"      文档状态: {completed} 已完成, {processing} 处理中, {failed} 失败 / 共 {len(statuses)}")

            # 有失败 → 立即报错退出，不再等待
            if failed > 0:
                print("\n      [ERROR] 文档处理失败！可能原因：")
                print("             1. milvus 未配置或未启动（config.yaml → milvus.address）")
                print("             2. LLM API Key 未设置（config.yaml → llm.providers.openai.api_key）")
                print("             3. Worker 进程未运行（需执行: go run ./cmd/worker）")
                print("             4. Redis 未连接（asynq 队列依赖 Redis）")
                print(f"\n      失败的文档:")
                for fd in failed_docs:
                    print(fd)
                return False

            if processing == 0 and completed > 0:
                print(f"      全部 {completed} 个文档处理完成 ✓")
                return True

            first_check = False
            time.sleep(5)  # 每 5 秒轮询一次

        # 超时但仍有 processing 的文档
        if processing > 0:
            print(f"\n      [WARN] 等待 {timeout}s 超时，{processing} 个文档仍在处理中")
            print("             请检查 worker 是否正常运行: go run ./cmd/worker")
            print("             或检查 Redis / Milvus 连接是否正常")
        return False

    def create_session(self, notebook_id: str, title: str = "Eval Session") -> str:
        """创建聊天会话"""
        resp = self._request("POST", f"/notebooks/{notebook_id}/sessions", {"title": title})
        session = resp.get("session", resp)
        # Go 默认序列化字段名为大写（无 json tag），兼容两种格式
        sid = session.get("ID") or session.get("id")
        if not sid:
            raise KeyError(f"session id not found in response: {list(session.keys())}")
        self.session_id = sid
        return self.session_id

    def chat_stream(self, notebook_id: str, session_id: str, question: str, timeout: int = 120) -> Dict:
        """
        发起流式问答并收集完整响应

        返回: {answer, sources, latency_ms, token_usage}
        """
        import http.client

        start_time = time.time()
        url_path = f"/api/v1/notebooks/{notebook_id}/sessions/{session_id}/chat"
        parsed = urllib.parse.urlparse(self.base_url)

        conn = http.client.HTTPConnection(parsed.hostname, port=parsed.port or 80, timeout=timeout)
        headers = self._headers()

        full_answer = ""
        sources = []

        try:
            conn.request("POST", url_path,
                        body=json.dumps({"question": question}).encode(),
                        headers=headers)
            resp = conn.getresponse()

            if resp.status != 200:
                body = resp.read().decode()
                raise Exception(f"SSE 请求失败 HTTP {resp.status}: {body}")

            # 处理 SSE 流
            for line in resp:
                line = line.decode('utf-8').strip()
                if not line.startswith('data: '):
                    continue
                payload = line[6:]
                if payload.strip() == '[DONE]':
                    break
                try:
                    event_data = json.loads(payload)
                    content = event_data.get('content', '')
                    if content:
                        full_answer += content
                    srcs = event_data.get('sources', [])
                    if srcs and not sources:
                        sources = [{
                            'document_id': s.get('document_id', ''),
                            'document_name': s.get('document_name', ''),
                            'page_number': s.get('page_number', 0),
                            'content': s.get('content', '')[:200],
                            'score': s.get('score', 0),
                        } for s in srcs]
                except json.JSONDecodeError:
                    pass

            latency_ms = int((time.time() - start_time) * 1000)

            return {
                'answer': full_answer.strip(),
                'sources': sources,
                'latency_ms': latency_ms,
                'token_usage': {
                    'prompt_tokens': estimate_tokens(question),
                    'completion_tokens': estimate_tokens(full_answer),
                    'total_tokens': estimate_tokens(question) + estimate_tokens(full_answer),
                }
            }
        finally:
            conn.close()


class LLMJudge:
    """LLM-as-Judge 评估器"""

    JUDGE_PROMPT_TEMPLATE = """你是一个专业的 RAG（检索增强生成）系统质量评估专家。请根据以下标准对 AI 的回答进行严格评估。

## 评估维度与标准

### 1. Groundedness (忠实度) [0-5分]
回答是否严格基于提供的参考答案/上下文？
- 5: 完全基于上下文，无任何额外信息
- 4: 基于上下文，有极少量合理推断
- 3: 大部分基于上下文，但有一些无关内容
- 2: 部分基于上下文，但有明显编造嫌疑
- 1: 大量编造或无关内容
- 0: 完全没有依据或答非所问

### 2. Relevance (相关性) [0-5分]
回答是否直接针对用户的问题？
- 5: 完全针对问题，精准回答
- 4: 针对问题，有小量冗余
- 3: 基本相关，但有些偏题
- 2: 部分相关，但偏离重点
- 1: 相关性很弱
- 0: 完全不相关

### 3. Completeness (完整性) [0-5分]
回答是否完整覆盖了问题的所有方面？
- 5: 完整覆盖所有要点
- 4: 覆盖了大部分要点
- 3: 覆盖了主要要点，缺少细节
- 2: 只覆盖部分要点
- 1: 只涉及很小一部分
- 0: 未回答或完全遗漏

### 4. Citation Precision (引用精确度) [0-1]
回答中的引用是否准确？（如果有引用的话）
- 1.0: 所有引用都正确指向有效来源
- 0.5: 部分引用有问题
- 0.0: 无引用或引用全部错误

### 5. Hallucination Detection (幻觉检测)
回答中是否存在以下类型的幻觉：
- 事实幻觉：编造文档中不存在的事实、数字、日期
- 来源幻觉：引用了不存在的文档或页面
- 逻辑幻觉：进行了无根据的推理或因果推断

## 待评估内容

### 用户问题：
{question}

### 参考答案（期望）：
{expected_answer}

### AI 实际回答：
{actual_answer}

### AI 引用的来源：
{sources_info}

## 输出格式（必须严格遵循此 JSON 格式）

```json
{{
  "groundedness_score": <0-5>,
  "relevance_score": <0-5>,
  "completeness_score": <0-5>,
  "citation_precision": <0.0-1.0>,
  "has_hallucination": <true/false>,
  "hallucination_details": "<描述检测到的幻觉，如果没有则写'未检测到明显幻觉'>",
  "reasoning": "<简述你的评判理由，包括扣分点>"
}}
```

请仅输出 JSON，不要包含其他内容："""

    def __init__(self, model: str, api_key: str, api_url: str):
        self.model = model
        self.api_key = api_key
        self.api_url = api_url.rstrip('/')

    def evaluate(self, question: str, expected: str, actual: str, sources: List[Dict]) -> Dict:
        """执行单次评估"""
        sources_str = "\n".join([
            f"- [{s.get('document_name', 'Unknown')}] Page {s.get('page_number', '?')}: "
            f"{s.get('content', '')[:150]}..."
            for s in (sources or [])
        ]) if sources else "(无引用来源)"

        prompt = self.JUDGE_PROMPT_TEMPLATE.format(
            question=question,
            expected_answer=expected,
            actual_answer=actual if actual else "(AI未生成回答)",
            sources_info=sources_str if sources_str else "(无来源)"
        )

        response = self._call_llm(prompt)
        return self._parse_response(response)

    def _call_llm(self, prompt: str, max_retries: int = 3) -> str:
        """调用 LLM API"""
        url = f"{self.api_url}/chat/completions"
        payload = {
            "model": self.model,
            "messages": [
                {"role": "system", "content": "你是一个专业的 RAG 系统质量评估专家。请严格按照要求输出 JSON 格式。"},
                {"role": "user", "content": prompt}
            ],
            "temperature": 0.1,  # 低温度保证稳定性
            "max_tokens": 2000,
        }

        for attempt in range(max_retries):
            try:
                req = urllib.request.Request(
                    url,
                    data=json.dumps(payload).encode(),
                    headers={
                        "Content-Type": "application/json",
                        "Authorization": f"Bearer {self.api_key}"
                    },
                    method="POST"
                )

                with urllib.request.urlopen(req, timeout=120) as resp:
                    result = json.loads(resp.read().decode())
                    return result['choices'][0]['message']['content'].strip()

            except Exception as e:
                if attempt < max_retries - 1:
                    time.sleep(2 ** attempt)  # 指数退避
                else:
                    err_msg = str(e)
                    if "401" in err_msg or "Unauthorized" in err_msg:
                        raise Exception(
                            f"Judge LLM 认证失败 (HTTP 401)。\n"
                            f"  原因: API Key 无效或未设置。\n"
                            f"  解决方案:\n"
                            f"    1. 设置环境变量: $env:OPENAI_API_KEY='sk-xxx'\n"
                            f"    2. 或传参: --judge-api-key sk-xxx\n"
                            f"    3. 或跳过 Judge: --no-judge\n"
                            f"  当前 Key 长度: {len(self.api_key)} 字符 ({'有效' if len(self.api_key) > 10 else '可能为空'})"
                        )
                    raise Exception(f"Judge LLM 调用失败 (重试 {max_retries} 次): {e}")

    def _parse_response(self, response: str) -> Dict:
        """解析 LLM 返回的 JSON 结果"""
        # 提取 markdown 代码块中的 JSON
        if '```json' in response:
            start = response.index('```json') + 7
            end = response.index('```', start)
            response = response[start:end].strip()
        elif '```' in response:
            start = response.index('```') + 3
            end = response.index('```', start)
            response = response[start:end].strip()

        try:
            return json.loads(response)
        except json.JSONDecodeError:
            # 尝试修复常见问题
            cleaned = response.strip()
            if not cleaned.startswith('{'):
                idx = cleaned.find('{')
                if idx >= 0:
                    cleaned = cleaned[idx:]
            if not cleaned.endswith('}'):
                idx = cleaned.rfind('}')
                if idx >= 0:
                    cleaned = cleaned[:idx+1]

            try:
                return json.loads(cleaned)
            except json.JSONDecodeError:
                return {
                    "groundedness_score": 0,
                    "relevance_score": 0,
                    "completeness_score": 0,
                    "citation_precision": 0,
                    "has_hallucination": True,
                    "hallucination_details": f"无法解析 Judge 输出: {response[:200]}",
                    "reasoning": "Judge 输出解析失败",
                }


def calculate_recall_at_k(expected_sources: List[str], actual_sources: List[Dict]) -> float:
    """
    计算 Recall@K：期望文档中有多少被实际检索到

    Args:
        expected_sources: 期望的源文档名列表
        actual_sources: 实际检索到的源列表（含 document_name）
    Returns:
        recall值 [0-1]
    """
    if not expected_sources:
        return 1.0  # 无期望来源则视为完美

    actual_doc_names = set()
    for src in (actual_sources or []):
        name = src.get('document_name', src.get('document_id', ''))
        if name:
            actual_doc_names.add(name.lower())

    matched = sum(1 for exp in expected_sources
                  if any(exp.lower() in act or act in exp.lower()
                         for act in actual_doc_names))

    return matched / len(expected_sources)


def estimate_tokens(text: str) -> int:
    """粗略估算 token 数（英文约 4 字符/token，中文约 1.5 字符/token）"""
    if not text:
        return 0
    # 混合中英文估算
    ascii_count = sum(1 for c in text if ord(c) < 128)
    non_ascii_count = len(text) - ascii_count
    return int(ascii_count / 4 + non_ascii_count * 1.5)


def run_evaluation(args) -> EvalReport:
    """执行完整的离线评估流程"""
    print("=" * 60)
    print("  NotebookMind - LLM-as-a-Judge 离线评估")
    print("=" * 60)
    print(f"  时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"  数据集: {args.dataset}")
    print(f"  API: {args.api_base}")
    print(f"  Judge: {args.judge_model} @ {args.judge_api_url}")
    key_status = f"{len(args.judge_api_key)} 字符" if args.judge_api_key else "未设置"
    print(f"  Key:  {key_status} (Judge {'启用' if args.enable_judge else '禁用'})")
    # 显示 .env 加载状态
    if _loaded_envs:
        env_keys = [k for k in _loaded_envs if k not in ('PATH', 'HOME', 'USERPROFILE')]
        if env_keys:
            print(f"  .env: 已加载 ({', '.join(env_keys)} 等变量)")
    print("=" * 60)

    # 1. 加载评测集
    print("\n[1/5] 加载评测集...")
    items = load_dataset(args.dataset)
    print(f"      加载 {len(items)} 条测试项")

    # 初始化 API 客户端
    client = APIClient(args.api_base)

    # 2. 登录并准备环境
    print("\n[2/5] 准备测试环境...")
    if args.skip_auth or client.login():
        print("      认证成功")
    else:
        print("      [WARN] 认证跳过")

    # 获取或创建笔记本
    notebooks = client.get_notebooks()
    if notebooks:
        notebook_id = notebooks[0]
        print(f"      使用已有笔记本: {notebook_id[:8]}...")
    else:
        notebook_id = client.create_notebook()
        print(f"      创建新笔记本: {notebook_id[:8]}...")

    # 上传测试文档（确保 RAG 有数据可检索）
    print("\n[2b/5] 准备测试文档...")
    # 优先查找项目根目录的 tests/pdf/ 或 tmp/uploads/ 中的 PDF
    pdf_dirs = [
        os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "tests", "pdf"),
        os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "tmp", "uploads"),
        args.pdf_dir if hasattr(args, 'pdf_dir') and args.pdf_dir else "",
    ]
    uploaded_count = 0
    for d in pdf_dirs:
        if not d or not os.path.isdir(d):
            continue
        for f in sorted(os.listdir(d)):
            if f.lower().endswith('.pdf'):
                try:
                    fp = os.path.join(d, f)
                    resp = client.upload_document(fp, notebook_id)
                    did = resp.get("document") or resp.get("ID") or resp.get("id")
                    print(f"      已上传: {f} → {str(did)[:12] if did else 'ok'}")
                    uploaded_count += 1
                except Exception as e:
                    print(f"      [WARN] 上传失败 {f}: {e}")
        if uploaded_count > 0:
            break

    if uploaded_count == 0:
        print("      [WARN] 未找到测试 PDF 文件，评测结果将基于空文档（AI 无法检索到上下文）")
        print("             请在以下位置放置测试 PDF:")
        print("               - tests/pdf/")
        print("               - tmp/uploads/")
        print("             或使用 --pdf-dir <路径> 指定")
    else:
        print(f"      共上传 {uploaded_count} 个文档，等待处理完成...")
        client.wait_for_documents(timeout=args.timeout)

    # 创建会话
    session_id = client.create_session(notebook_id)
    print(f"      会话: {session_id[:8]}...")

    # 初始化 Judge
    print("\n[3/5] 初始化 LLM Judge...")
    judge = LLMJudge(
        model=args.judge_model,
        api_key=args.judge_api_key,
        api_url=args.judge_api_url,
    )
    print(f"      模型: {args.judge_model}")

    # 3. 执行评测
    print(f"\n[4/5] 执行评测 ({len(items)} 项)...")
    results: List[EvalResult] = []
    latencies = []

    for i, item in enumerate(items):
        print(f"\n      [{i+1}/{len(items)}] {item.id} ({item.category})")
        print(f"          Q: {item.question[:60]}...")

        result = EvalResult(
            item_id=item.id,
            category=item.category,
            question=item.question,
            expected_answer=item.expected_answer,
        )

        try:
            # 调用 API
            start_time = time.time()
            response = client.chat_stream(notebook_id, session_id, item.question, timeout=args.timeout)
            result.actual_answer = response['answer']
            result.sources = response['sources']
            result.latency_ms = response['latency_ms']
            result.prompt_tokens = response['token_usage']['prompt_tokens']
            result.completion_tokens = response['token_usage']['completion_tokens']
            result.total_tokens = response['token_usage']['total_tokens']

            latencies.append(result.latency_ms)

            print(f"          A: {result.actual_answer[:80]}...")
            print(f"          Latency: {result.latency_ms}ms | Sources: {len(result.sources)}")

        except Exception as e:
            result.error = str(e)
            results.error_count += 1
            print(f"          ERROR: {e}")

        # 计算检索召回率
        result.recall_at_k = calculate_recall_at_k(item.expected_sources, result.sources)

        # 4. 调用 Judge 评估（仅在 API 调用成功时）
        if result.actual_answer and args.enable_judge:
            try:
                print(f"          Judging...", end=" ", flush=True)
                judge_result = judge.evaluate(
                    question=item.question,
                    expected=item.expected_answer,
                    actual=result.actual_answer,
                    sources=result.sources,
                )
                result.groundedness_score = judge_result.get('groundedness_score', 0)
                result.relevance_score = judge_result.get('relevance_score', 0)
                result.completeness_score = judge_result.get('completeness_score', 0)
                result.citation_precision = judge_result.get('citation_precision', 0)
                result.has_hallucination = judge_result.get('has_hallucination', False)
                result.hallucination_details = judge_result.get('hallucination_details', '')
                result.judge_reasoning = judge_result.get('reasoning', '')

                # 计算综合分
                result.overall_score = (
                    result.groundedness_score * 30 +   # 30% 权重
                    result.relevance_score * 25 +      # 25%
                    result.completeness_score * 25 +    # 25%
                    result.citation_precision * 20      # 20%
                )  # 满分 100

                print(f"G={result.groundedness_score:.1f} R={result.relevance_score:.1f} "
                      f"C={result.completeness_score:.1f} CP={result.citation_precision:.2f} "
                      f"H={'Y' if result.has_hallucination else 'N'} O={result.overall_score:.1f}")

            except Exception as e:
                print(f"Judge Error: {e}")
                result.judge_reasoning = f"Judge 失败: {e}"

        results.append(result)

        # 速率控制
        time.sleep(args.delay)

    # 5. 生成报告
    print(f"\n[5/5] 生成报告...")

    report = generate_report(results, latencies)

    # 保存报告
    output_path = args.output or f"eval_results_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
    save_report(report, output_path)
    print(f"\n      报告已保存: {output_path}")

    # 打印摘要
    print_summary(report)

    return report


def generate_report(results: List[EvalResult], latencies: List[int]) -> EvalReport:
    """生成评测报告"""
    report = EvalReport(
        run_id=str(uuid.uuid4())[:8],
        timestamp=datetime.now().isoformat(),
        total_items=len(results),
        success_count=sum(1 for r in results if not r.error),
        error_count=sum(1 for r in results if r.error),
    )

    if latencies:
        sorted_lat = sorted(latencies)
        report.avg_latency_ms = sum(latencies) / len(latencies)
        report.p95_latency_ms = sorted_lat[int(len(sorted_lat) * 0.95)] if len(sorted_lat) > 1 else sorted_lat[-1]

    # 计算平均分
    judged = [r for r in results if r.overall_score > 0]
    if judged:
        report.avg_groundedness = sum(r.groundedness_score for r in judged) / len(judged)
        report.avg_relevance = sum(r.relevance_score for r in judged) / len(judged)
        report.avg_completeness = sum(r.completeness_score for r in judged) / len(judged)
        report.avg_citation_precision = sum(r.citation_precision for r in judged) / len(judged)
        report.avg_overall_score = sum(r.overall_score for r in judged) / len(judged)
        report.hallucination_rate = sum(1 for r in judged if r.has_hallucination) / len(judged)
        report.avg_recall_at_k = sum(r.recall_at_k for r in results if r.recall_at_k > 0) / max(len([r for r in results if r.recall_at_k >= 0]), 1)

    # 分类统计
    from collections import defaultdict
    cat_stats: Dict[str, Dict] = defaultdict(lambda: {'count': 0, 'avg_score': 0, 'scores': []})
    for r in results:
        cat_stats[r.category]['count'] += 1
        cat_stats[r.category]['scores'].append(r.overall_score)

    for cat, stats in cat_stats.items():
        valid_scores = [s for s in stats['scores'] if s > 0]
        report.category_stats[cat] = {
            'count': stats['count'],
            'avg_score': sum(valid_scores) / len(valid_scores) if valid_scores else 0,
        }

    report.results = results
    return report


def save_report(report: EvalReport, path: str):
    """保存报告到 JSON 文件"""
    with open(path, 'w', encoding='utf-8') as f:
        json.dump(asdict(report), f, ensure_ascii=False, indent=2)


def print_summary(report: EvalReport):
    """打印评估摘要"""
    print("\n" + "=" * 70)
    print("  NotebookMind 评估报告摘要")
    print("=" * 70)
    print(f"""
  基本信息
  ──────────────────────────────────────
  Run ID:       {report.run_id}
  Timestamp:    {report.timestamp}
  总测试项:      {report.total_items}
  成功:         {report.success_count}
  失败:         {report.error_count}

  性能指标
  ──────────────────────────────────────
  平均延迟:      {report.avg_latency_ms:.0f}ms
  P95延迟:       {report.p95_latency_ms:.0f}ms

  质量指标 (LLM-as-Judge)
  ──────────────────────────────────────
  Groundedness:  {report.avg_groundedness:.2f} / 5
  Relevance:     {report.avg_relevance:.2f} / 5
  Completeness:  {report.avg_completeness:.2f} / 5
  Citation Prec: {report.avg_citation_precision:.2f} / 1
  幻觉率:        {report.hallucination_rate:.1%}
  Recall@K:     {report.avg_recall_at_k:.1%}
  
  综合评分:     {report.avg_overall_score:.1f} / 100
""")

    if report.category_stats:
        print("  分类统计")
        print("  ──────────────────────────────────────")
        for cat, stats in report.category_stats.items():
            print(f"  {cat:<22s} n={stats['count']:>3}  avg={stats['avg_score']:.1f}")

    print("=" * 70)


def main():
    parser = argparse.ArgumentParser(
        description="NotebookMind LLM-as-a-Judge 离线评估工具",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  python notebook_eval.py
  python notebook_eval.py --dataset ../tests/data/eval_dataset.jsonl --enable-judge
  python notebook_eval.py --judge-model gpt-4o --judge-api-key sk-xxx
        """,
    )

    parser.add_argument('--dataset', '-d',
                       default=os.path.join(os.path.dirname(__file__), '..', 'tests', 'data', 'eval_dataset.jsonl'),
                       help='评测集路径 (JSONL格式)')
    parser.add_argument('--output', '-o',
                       default='',
                       help='输出报告路径 (默认: eval_results_TIMESTAMP.json)')
    # API 基础 URL：从 .env 的端口推导或使用 NEXT_PUBLIC_API_URL
    _port = os.environ.get('PORT', os.environ.get('APP_PORT', '8080'))
    _api_url = os.environ.get('NEXT_PUBLIC_API_URL', f'http://localhost:{_port}/api/v1')
    parser.add_argument('--api-base',
                       default=os.environ.get('EVAL_API_BASE', _api_url),
                       help='API 基础 URL (默认从 .env 读取)')
    parser.add_argument('--skip-auth',
                       action='store_true',
                       help='跳过认证')
    parser.add_argument('--enable-judge',
                       action='store_true',
                       default=True,
                       help='启用 LLM Judge 评估 (默认启用)')
    parser.add_argument('--no-judge',
                       action='store_true',
                       help='禁用 LLM Judge (只测 API 可用性和延迟)')

    # Judge 配置：优先从 .env 的 OPENAI_* 读取
    _default_judge_key = (
        os.environ.get('EVAL_JUDGE_API_KEY')           # 显式设置
        or os.environ.get('OPENAI_API_KEY', '')         # 项目 .env
    )
    _default_judge_url = (
        os.environ.get('EVAL_JUDGE_API_URL')            # 显式设置
        or os.environ.get('OPENAI_BASE_URL', 'https://api.openai.com/v1')  # 项目 .env
        if os.environ.get('OPENAI_BASE_URL')
        else 'https://api.openai.com/v1'
    )
    parser.add_argument('--judge-model',
                       default=os.environ.get('EVAL_JUDGE_MODEL', 'gpt-4o-mini'),
                       help='Judge 模型名称 (默认 gpt-4o-mini)')
    parser.add_argument('--judge-api-key',
                       default=_default_judge_key,
                       help='Judge API Key (默认从 .env OPENAI_API_KEY 读取)')
    parser.add_argument('--judge-api-url',
                       default=_default_judge_url,
                       help='Judge API URL (默认从 .env OPENAI_BASE_URL 读取)')
    parser.add_argument('--timeout', '-t',
                       type=int, default=120,
                       help='单个请求超时时间(秒)')
    parser.add_argument('--delay',
                       type=float, default=0.5,
                       help='请求间隔(秒)')
    parser.add_argument('--verbose', '-v',
                       action='store_true',
                       help='详细输出')
    parser.add_argument('--pdf-dir',
                       default='',
                       type=str,
                       help='测试 PDF 文件目录（默认自动查找 tests/pdf/ 或 tmp/uploads/）')

    args = parser.parse_args()

    if args.no_judge:
        args.enable_judge = False

    if args.enable_judge and not args.judge_api_key:
        print("[ERROR] 启用 Judge 但未提供有效的 API Key")
        print("       请通过以下方式提供（按优先级）:")
        print("       1. 命令行参数: --judge-api-key sk-xxx")
        print("       2. 项目 .env 文件: OPENAI_API_KEY=sk-xxx (项目根目录)")
        print("       3. 环境变量: EVAL_JUDGE_API_KEY=sk-xxx")
        print("       4. 环境变量: OPENAI_API_KEY=sk-xxx")
        print(f"       当前 .env 路径: {os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '.env')}")
        print("       或使用 --no-judge 禁用 Judge 评估")
        sys.exit(1)

    try:
        report = run_evaluation(args)

        # 返回退出码
        if report.error_count > report.success_count // 2:
            sys.exit(2)  # 大量失败
        elif report.avg_overall_score < 50:
            sys.exit(1)  # 质量偏低
        else:
            sys.exit(0)  # 成功

    except KeyboardInterrupt:
        print("\n\n[INFO] 用户中断评估")
        sys.exit(130)
    except Exception as e:
        print(f"\n[FATAL] 评估异常: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == '__main__':
    main()
