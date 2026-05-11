BEGIN;

-- 可重复执行：先删联调样例
DELETE FROM t_rag_trace_node WHERE trace_id IN ('trace_bad_01', 'trace_tool_01');
DELETE FROM t_rag_trace_run  WHERE trace_id IN ('trace_bad_01', 'trace_tool_01');

DELETE FROM t_ingestion_task_node WHERE task_id IN ('task_fail_01', 'task_run_01', 'task_ok_01');
DELETE FROM t_ingestion_task      WHERE id      IN ('task_fail_01', 'task_run_01', 'task_ok_01');

DELETE FROM t_knowledge_document_chunk_log WHERE id IN ('task_fail_01', 'task_run_01', 'task_ok_01');
DELETE FROM t_knowledge_document           WHERE id IN ('doc_fail_01', 'doc_run_01', 'doc_ok_01');

DELETE FROM t_ingestion_pipeline WHERE id IN ('pipe_demo_01');
DELETE FROM t_knowledge_base     WHERE id IN ('kb_demo_01');

-- 基础数据
INSERT INTO t_knowledge_base (
  id, name, embedding_model, collection_name, created_by, updated_by, deleted
) VALUES (
  'kb_demo_01', '联调知识库', 'text-embedding-3-large', 'kb_demo_01', 'u_demo_01', 'u_demo_01', 0
);

INSERT INTO t_ingestion_pipeline (
  id, name, description, nodes_json, created_by, updated_by, deleted
) VALUES (
  'pipe_demo_01',
  '联调导入流水线',
  '用于 agentic chat 联调的固定样例流水线',
  '[
    {"id":"fetcher","type":"fetcher","order":1},
    {"id":"parser","type":"parser","order":2},
    {"id":"chunker","type":"chunker","order":3},
    {"id":"indexer","type":"indexer","order":4}
  ]'::jsonb,
  'u_demo_01',
  'u_demo_01',
  0
);

-- 文档：失败样例
INSERT INTO t_knowledge_document (
  id, kb_id, doc_name, enabled, chunk_count, file_url, file_type,
  file_size, process_mode, status, source_type, source_location,
  pipeline_id, created_by, updated_by, deleted
) VALUES (
  'doc_fail_01', 'kb_demo_01', '失败样例文档.pdf', 1, 0,
  'knowledge/kb_demo_01/documents/doc_fail_01/fail.pdf',
  'pdf', 102400, 'pipeline', 'failed', 'file', 'D:/samples/fail.pdf',
  'pipe_demo_01', 'u_demo_01', 'u_demo_01', 0
);

INSERT INTO t_ingestion_task (
  id, pipeline_id, source_type, source_location, source_file_name,
  status, chunk_count, error_message, metadata, started_at, completed_at,
  created_by, updated_by, deleted
) VALUES (
  'task_fail_01',
  'pipe_demo_01',
  'file',
  'D:/samples/fail.pdf',
  '失败样例文档.pdf',
  'failed',
  0,
  'indexer failed after retries',
  '{
    "documentId":"doc_fail_01",
    "knowledgeBaseId":"kb_demo_01",
    "documentName":"失败样例文档.pdf"
  }'::jsonb,
  NOW() - INTERVAL '12 minutes',
  NOW() - INTERVAL '10 minutes',
  'u_demo_01',
  'u_demo_01',
  0
);

INSERT INTO t_ingestion_task_node (
  id, task_id, pipeline_id, node_id, node_type, node_order,
  status, duration_ms, message, error_message, output, deleted
) VALUES
(
  'tn_fail_001', 'task_fail_01', 'pipe_demo_01', 'fetcher', 'fetcher', 1,
  'success', 320, 'source fetched', NULL,
  '{"bytes":102400}'::jsonb, 0
),
(
  'tn_fail_002', 'task_fail_01', 'pipe_demo_01', 'parser', 'parser', 2,
  'success', 980, 'text parsed', NULL,
  '{"pages":12,"chars":18420}'::jsonb, 0
),
(
  'tn_fail_003', 'task_fail_01', 'pipe_demo_01', 'chunker', 'chunker', 3,
  'success', 410, 'chunks built', NULL,
  '{"chunkCount":37}'::jsonb, 0
),
(
  'tn_fail_004', 'task_fail_01', 'pipe_demo_01', 'indexer', 'indexer', 4,
  'failed', 5210, 'indexing failed after retries',
  'connection refused: vector store unavailable',
  '{"retryCount":3,"target":"pgvector"}'::jsonb, 0
);

INSERT INTO t_knowledge_document_chunk_log (
  id, doc_id, status, process_mode, chunk_strategy, pipeline_id,
  extract_duration, chunk_duration, embed_duration, persist_duration,
  total_duration, chunk_count, error_message, start_time, end_time
) VALUES (
  'task_fail_01',
  'doc_fail_01',
  'failed',
  'pipeline',
  NULL,
  'pipe_demo_01',
  980,
  410,
  1800,
  5210,
  8400,
  0,
  'indexer failed after retries',
  NOW() - INTERVAL '12 minutes',
  NOW() - INTERVAL '10 minutes'
);

-- 文档：运行中样例
INSERT INTO t_knowledge_document (
  id, kb_id, doc_name, enabled, chunk_count, file_url, file_type,
  file_size, process_mode, status, source_type, source_location,
  pipeline_id, created_by, updated_by, deleted
) VALUES (
  'doc_run_01', 'kb_demo_01', '运行中文档.md', 1, 0,
  'knowledge/kb_demo_01/documents/doc_run_01/run.md',
  'md', 20480, 'pipeline', 'running', 'file', 'D:/samples/run.md',
  'pipe_demo_01', 'u_demo_01', 'u_demo_01', 0
);

INSERT INTO t_ingestion_task (
  id, pipeline_id, source_type, source_location, source_file_name,
  status, chunk_count, error_message, metadata, started_at, completed_at,
  created_by, updated_by, deleted
) VALUES (
  'task_run_01',
  'pipe_demo_01',
  'file',
  'D:/samples/run.md',
  '运行中文档.md',
  'running',
  12,
  NULL,
  '{
    "documentId":"doc_run_01",
    "knowledgeBaseId":"kb_demo_01",
    "documentName":"运行中文档.md"
  }'::jsonb,
  NOW() - INTERVAL '4 minutes',
  NULL,
  'u_demo_01',
  'u_demo_01',
  0
);

INSERT INTO t_ingestion_task_node (
  id, task_id, pipeline_id, node_id, node_type, node_order,
  status, duration_ms, message, error_message, output, deleted
) VALUES
(
  'tn_run_001', 'task_run_01', 'pipe_demo_01', 'fetcher', 'fetcher', 1,
  'success', 180, 'source fetched', NULL,
  '{"bytes":20480}'::jsonb, 0
),
(
  'tn_run_002', 'task_run_01', 'pipe_demo_01', 'parser', 'parser', 2,
  'success', 450, 'text parsed', NULL,
  '{"chars":8200}'::jsonb, 0
),
(
  'tn_run_003', 'task_run_01', 'pipe_demo_01', 'chunker', 'chunker', 3,
  'success', 160, 'chunks built', NULL,
  '{"chunkCount":12}'::jsonb, 0
),
(
  'tn_run_004', 'task_run_01', 'pipe_demo_01', 'indexer', 'indexer', 4,
  'running', 2600, 'embedding and persist in progress', NULL,
  '{"progress":"12/37"}'::jsonb, 0
);

INSERT INTO t_knowledge_document_chunk_log (
  id, doc_id, status, process_mode, chunk_strategy, pipeline_id,
  extract_duration, chunk_duration, embed_duration, persist_duration,
  total_duration, chunk_count, error_message, start_time, end_time
) VALUES (
  'task_run_01',
  'doc_run_01',
  'running',
  'pipeline',
  NULL,
  'pipe_demo_01',
  450,
  160,
  1100,
  NULL,
  3200,
  12,
  NULL,
  NOW() - INTERVAL '4 minutes',
  NULL
);

-- 文档：成功样例
INSERT INTO t_knowledge_document (
  id, kb_id, doc_name, enabled, chunk_count, file_url, file_type,
  file_size, process_mode, status, source_type, source_location,
  pipeline_id, created_by, updated_by, deleted
) VALUES (
  'doc_ok_01', 'kb_demo_01', '成功样例文档.txt', 1, 25,
  'knowledge/kb_demo_01/documents/doc_ok_01/ok.txt',
  'txt', 5120, 'pipeline', 'success', 'file', 'D:/samples/ok.txt',
  'pipe_demo_01', 'u_demo_01', 'u_demo_01', 0
);

INSERT INTO t_ingestion_task (
  id, pipeline_id, source_type, source_location, source_file_name,
  status, chunk_count, error_message, metadata, started_at, completed_at,
  created_by, updated_by, deleted
) VALUES (
  'task_ok_01',
  'pipe_demo_01',
  'file',
  'D:/samples/ok.txt',
  '成功样例文档.txt',
  'success',
  25,
  NULL,
  '{
    "documentId":"doc_ok_01",
    "knowledgeBaseId":"kb_demo_01",
    "documentName":"成功样例文档.txt"
  }'::jsonb,
  NOW() - INTERVAL '20 minutes',
  NOW() - INTERVAL '18 minutes',
  'u_demo_01',
  'u_demo_01',
  0
);

INSERT INTO t_ingestion_task_node (
  id, task_id, pipeline_id, node_id, node_type, node_order,
  status, duration_ms, message, error_message, output, deleted
) VALUES
(
  'tn_ok_001', 'task_ok_01', 'pipe_demo_01', 'fetcher', 'fetcher', 1,
  'success', 100, 'source fetched', NULL,
  '{"bytes":5120}'::jsonb, 0
),
(
  'tn_ok_002', 'task_ok_01', 'pipe_demo_01', 'parser', 'parser', 2,
  'success', 220, 'text parsed', NULL,
  '{"chars":2600}'::jsonb, 0
),
(
  'tn_ok_003', 'task_ok_01', 'pipe_demo_01', 'chunker', 'chunker', 3,
  'success', 90, 'chunks built', NULL,
  '{"chunkCount":25}'::jsonb, 0
),
(
  'tn_ok_004', 'task_ok_01', 'pipe_demo_01', 'indexer', 'indexer', 4,
  'success', 880, 'index finished', NULL,
  '{"persisted":25}'::jsonb, 0
);

INSERT INTO t_knowledge_document_chunk_log (
  id, doc_id, status, process_mode, chunk_strategy, pipeline_id,
  extract_duration, chunk_duration, embed_duration, persist_duration,
  total_duration, chunk_count, error_message, start_time, end_time
) VALUES (
  'task_ok_01',
  'doc_ok_01',
  'success',
  'pipeline',
  NULL,
  'pipe_demo_01',
  220,
  90,
  430,
  450,
  1190,
  25,
  NULL,
  NOW() - INTERVAL '20 minutes',
  NOW() - INTERVAL '18 minutes'
);

-- trace：召回差样例
INSERT INTO t_rag_trace_run (
  id, trace_id, trace_name, entry_method, conversation_id, task_id, user_id,
  status, error_message, start_time, end_time, duration_ms, extra_data,
  create_time, update_time, deleted
) VALUES (
  'trun_bad_001',
  'trace_bad_01',
  'rag_chat_trace',
  '/rag/v3/chat',
  'conv_demo_01',
  'ragtask_001',
  'u_demo_01',
  'success',
  NULL,
  NOW() - INTERVAL '6 minutes',
  NOW() - INTERVAL '6 minutes' + INTERVAL '900 milliseconds',
  900,
  '{"question":"这个知识库里关于 embedding 的说明是什么？"}',
  NOW(),
  NOW(),
  0
);

INSERT INTO t_rag_trace_node (
  id, trace_id, node_id, parent_node_id, depth, node_type, node_name,
  class_name, method_name, status, error_message, start_time, end_time,
  duration_ms, extra_data, create_time, update_time, deleted
) VALUES
(
  'tnode_b001', 'trace_bad_01', 'rewrite', NULL, 1, 'rewrite', 'query_rewrite',
  'RewriteService', 'Rewrite', 'success', NULL,
  NOW() - INTERVAL '6 minutes',
  NOW() - INTERVAL '6 minutes' + INTERVAL '60 milliseconds',
  60,
  '{"rewrittenQuery":"embedding 配置 说明"}',
  NOW(), NOW(),
  0
),
(
  'tnode_b002', 'trace_bad_01', 'retrieve', NULL, 1, 'retrieve', 'hybrid_retrieve',
  'RetrieveService', 'Retrieve', 'success', NULL,
  NOW() - INTERVAL '6 minutes' + INTERVAL '60 milliseconds',
  NOW() - INTERVAL '6 minutes' + INTERVAL '300 milliseconds',
  240,
  '{"chunkCount":0,"searchMode":"hybrid","topScore":0.18}',
  NOW(), NOW(),
  0
),
(
  'tnode_b003', 'trace_bad_01', 'prompt', NULL, 1, 'prompt', 'build_messages',
  'PromptService', 'Build', 'success', NULL,
  NOW() - INTERVAL '6 minutes' + INTERVAL '300 milliseconds',
  NOW() - INTERVAL '6 minutes' + INTERVAL '420 milliseconds',
  120,
  '{"toolContextUsed":false}',
  NOW(), NOW(),
  0
);

-- trace：tool workflow degraded 样例
INSERT INTO t_rag_trace_run (
  id, trace_id, trace_name, entry_method, conversation_id, task_id, user_id,
  status, error_message, start_time, end_time, duration_ms, extra_data,
  create_time, update_time, deleted
) VALUES (
  'trun_tool001',
  'trace_tool_01',
  'rag_chat_trace',
  '/rag/v3/chat',
  'conv_demo_02',
  'ragtask_002',
  'u_demo_01',
  'success',
  NULL,
  NOW() - INTERVAL '3 minutes',
  NOW() - INTERVAL '3 minutes' + INTERVAL '1400 milliseconds',
  1400,
  '{"question":"doc_fail_01 为什么失败了？"}',
  NOW(),
  NOW(),
  0
);

INSERT INTO t_rag_trace_node (
  id, trace_id, node_id, parent_node_id, depth, node_type, node_name,
  class_name, method_name, status, error_message, start_time, end_time,
  duration_ms, extra_data, create_time, update_time, deleted
) VALUES
(
  'tnode_t001', 'trace_tool_01', 'rewrite', NULL, 1, 'rewrite', 'query_rewrite',
  'RewriteService', 'Rewrite', 'success', NULL,
  NOW() - INTERVAL '3 minutes',
  NOW() - INTERVAL '3 minutes' + INTERVAL '80 milliseconds',
  80,
  '{"rewrittenQuery":"doc_fail_01 导入失败原因"}',
  NOW(), NOW(),
  0
),
(
  'tnode_t002', 'trace_tool_01', 'retrieve', NULL, 1, 'retrieve', 'hybrid_retrieve',
  'RetrieveService', 'Retrieve', 'success', NULL,
  NOW() - INTERVAL '3 minutes' + INTERVAL '80 milliseconds',
  NOW() - INTERVAL '3 minutes' + INTERVAL '260 milliseconds',
  180,
  '{"chunkCount":4,"searchMode":"hybrid","topScore":0.91}',
  NOW(), NOW(),
  0
),
(
  'tnode_t003', 'trace_tool_01', 'tool_workflow', NULL, 1, 'tool', 'agent_loop',
  'AgentLoop', 'Run', 'success', NULL,
  NOW() - INTERVAL '3 minutes' + INTERVAL '260 milliseconds',
  NOW() - INTERVAL '3 minutes' + INTERVAL '900 milliseconds',
  640,
  '{"toolCallCount":2,"degraded":true,"degradeReason":"task lookup failed","toolNames":["document_query","task_ingestion_diagnose"]}',
  NOW(), NOW(),
  0
),
(
  'tnode_t004', 'trace_tool_01', 'prompt', NULL, 1, 'prompt', 'build_messages',
  'PromptService', 'Build', 'success', NULL,
  NOW() - INTERVAL '3 minutes' + INTERVAL '900 milliseconds',
  NOW() - INTERVAL '3 minutes' + INTERVAL '1040 milliseconds',
  140,
  '{"toolContextUsed":true}',
  NOW(), NOW(),
  0
);

COMMIT;
