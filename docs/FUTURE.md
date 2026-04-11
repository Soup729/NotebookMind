在后端功能完善后，gemini3pro一键优化所有前端页，文档摘要下的建议问题不是根据文档生成的，而且点击了不能输入到对话框。

北极星目标（最终状态）
在现有项目上实现与 NotebookLM 接近的三类核心能力：

- 高质量文档理解：复杂 PDF/图表/表格/扫描件可稳定入库并可检索。
- 高可信问答：答案有来源、可追溯、低幻觉、支持跨文档综合推理。
- Notebook 工作流：总结、FAQ、对比、洞察、引用定位、长会话记忆形成闭环。

1. 待测试：Phase 0（第1-2周）：基线与质量护栏先行
- 改进目标
    建立“可衡量”的质量体系，避免后续优化无标准。

- 具体实现：
    新建标准的JSONL格式离线评测集：覆盖事实问答、跨文档对比、表格问答、图文问答、多轮追问等多场景。
    增加在线指标采集：检索召回、引用命中率、幻觉率、p95 延迟、token 成本。
    为了收集在线指标（p95 延迟、Token 成本、检索召回率），需要对关键链路埋点：chat_service、notebook_chat_service、worker/processor、notebook_service。注入结构化日志 (Structured Logging)。借助 go.uber.org/zap。
    在scripts文件夹下新建一个notebook_eval.py文件实现离线评估脚本，使用 LLM-as-a-Judge（大模型作为裁判） 的思想，通过一个 Python 脚本自动调用您的本地 Go API，并评估回答质量。
    同步更新README和tests/README

- 实现效果：
    形成固定周报：Recall@K、Groundedness、Citation Precision、用户反馈分。
    后续每次改动都能量化“提升/退化”。

- 测试：
    python scripts/notebook_eval.py --api-base http://localhost:8080/api/v1 --judge-model gpt-4o


2. 待测试：Phase 1（第3-6周）：文档解析与入库重构（当前最大短板）
- 改进目标：
    把“纯文本抽取+字符分块”升级为“结构化文档理解”。

- 具体实现：重构解析链路：以 parser_service 为主入口，统一替换当前 worker/processor 的简化抽取流程。
    补齐能力：
    OCR（扫描件）RapidOCR
    表格结构化（行列、表头、单元格）
    图片块描述（caption）与图表文本化，配置VLM实现，在环境里添加相关配置模板，如果没有配置的话降级不使用。
    标题层级、段落、页码、bbox 全量保留
    实现父子块策略：child 用于召回，parent 用于回答上下文拼接。
    入库元数据标准化：document_id/page/chunk_type/bbox/section/path。
    同步更新README
    实现效果：
    复杂文档可读性显著提升，表格问答准确率提高。
    引用可定位到页码/段落（同时前端高亮可直接消费）。

3. Phase 2（第7-10周）：检索系统升级为生产级 Hybrid RAG
- 改进目标：
    把“可选混合检索框架”变为主链路默认能力。

- 具体实现：将 hybrid_search 接入 chat_service 与 notebook_chat_service 默认检索路径。
    修复并强化 BM25：正确维护 avgDL/DF/IDF，默认采用 向量 (Dense) + BM25 (Sparse) 双路召回
    中英文混合分词：交由 gojieba.CutForSearch 引擎，它内置了 HMM 隐马尔可夫模型，能很好地切分中英混合句子。
    停用词/词形归一：在内存中维护一个 map[string]struct{} 的 HashSet，实现在 O(1) 时间复杂度内的快速过滤；依赖 strings.ToLower() 处理大小写，依赖 golang.org/x/text 剥离欧洲语言的重音符。
    引入 reranker（cross-encoder）用于 TopN 重排，使用coherent api，在配置里添加相关配置模板，如果没有配置的话降级不使用。
    加入 query rewrite + 意图路由（事实、总结、对比、流程等不同检索策略）。
    增加 failover：检索置信度低时自动二次检索（扩展 query / 放宽过滤）。
    增加兜底策略，避免陷入堵塞
    同步更新README

- 实现效果：
    采用轻量级实现，在提升检索效果的同时保持响应速度
    跨文档问题召回显著提升，错误引用减少。
    对“长尾问题”和“模糊提问”更稳健。

4. Phase 3（第11-14周）：答案生成可信化（从“能答”到“可托付”）
- 改进目标：
    建立答案质量闭环：可解释、可反思、可纠错。

    升级 agent_workflow：
    从占位逻辑改为真实 Planner→Retrieve→Reason→Verify 流程
    增加 citation checker（答案每段必须能回溯证据）
    升级 GenerateReflection：
    结构化 JSON 解析（严格 schema）
    与回答后处理联动（自动补检索/重写回答）
    增加“拒答/降级策略”：
    证据不足时明确声明不确定，而非强答。
    同步更新README

- 实现效果
    幻觉率明显下降。
    回答可信度、引用覆盖度接近 NotebookLM 体验。

5. Phase 4（第15-18周）：多模态 Notebook RAG 打通
- 改进目标
    把现有 VQA 能力从“单图问答”升级为“文档级多模态问答”。

- 具体实现：图片、图表、表格在入库阶段统一生成可检索语义表示。
    检索阶段支持文本块+视觉块联合召回与重排。
    生成阶段支持“图文联合证据回答”，并输出多模态引用信息。
    同步更新README

-  实现效果
    对图表型资料（财报、论文、报告）问答质量接近 NotebookLM 核心优势场景。

6. Phase 5（第19-22周）：NotebookLM 风格能力补齐

- 改进目标
    从“问答系统”升级为“研究工作台”。

- 具体实现：
    Notebook 级能力：
    跨文档对比结论
    主题聚类与时间线
    自动提纲/学习指南/FAQ/关键洞察
    长会话记忆：
    会话摘要压缩
    用户偏好与术语记忆
    任务化输出：
    一键生成 briefing、复习卡片、会议摘要草稿等。
    同步更新README
    
- 实现效果
    用户在一个 notebook 内形成持续研究闭环，体验接近 NotebookLM 的“工作流价值”。

Phase 6（第23-24周）：企业级稳定性与租户治理
改进目标
补齐生产级能力，支撑规模化使用。

- 具体实现：
    强化 tenant_service（当前校验逻辑过于简化）：
    资源级 ACL、角色权限、审计日志、配额与限流
    增加鲁棒性：
    幂等任务、重试策略、死信处理、故障回放
    成本优化：
    分层模型路由（轻量模型优先，重模型兜底）
    缓存与结果复用策略

- 实现效果
    多租户安全、性能、成本可控，满足企业生产要求。
    建议的“验收指标”（是否达到 NotebookLM 级体验）
    引用准确率（Citation Precision）≥ 90%
    幻觉率（无证据断言）≤ 5%
    跨文档综合问答正确率 ≥ 85%
    表格/图表问答正确率 ≥ 80%
    首 token 延迟 p95 ≤ 2.5s，完整回答 p95 ≤ 12s
    用户侧满意度（thumb up）≥ 80%
