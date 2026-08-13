-- 加宽 chunk id 列：父子分块下父块 id 形如 {docId}-p-{idx}，雪花 docId 较长时会超过 varchar(20)。
ALTER TABLE t_knowledge_chunk ALTER COLUMN id TYPE VARCHAR(64);
ALTER TABLE t_knowledge_chunk_vector ALTER COLUMN chunk_id TYPE VARCHAR(64);
