-- Wiki 页面表
CREATE TABLE IF NOT EXISTS t_wiki_page (
    id              VARCHAR(20) PRIMARY KEY,
    kb_id           VARCHAR(20) NOT NULL,
    slug            VARCHAR(256) NOT NULL,
    title           VARCHAR(256) NOT NULL,
    page_type       VARCHAR(16) NOT NULL DEFAULT 'entity',
    status          VARCHAR(16) NOT NULL DEFAULT 'published',
    content         TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    source_document_ids JSONB NOT NULL DEFAULT '[]',
    source_chunk_ids    JSONB NOT NULL DEFAULT '[]',
    created_by      VARCHAR(20) NOT NULL,
    updated_by      VARCHAR(20) NOT NULL,
    create_time     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT NOT NULL DEFAULT 0
);
-- 软删除场景下保证 (kb_id, slug) 唯一：仅对未删除行生效
CREATE UNIQUE INDEX IF NOT EXISTS uk_wiki_page_kb_slug_active ON t_wiki_page (kb_id, slug) WHERE deleted = 0;
CREATE INDEX IF NOT EXISTS idx_wiki_page_kb ON t_wiki_page (kb_id);

-- Wiki 页面间链接表
CREATE TABLE IF NOT EXISTS t_wiki_link (
    id              VARCHAR(20) PRIMARY KEY,
    kb_id           VARCHAR(20) NOT NULL,
    from_page_id    VARCHAR(256) NOT NULL,
    to_page_id      VARCHAR(256),
    target_type     VARCHAR(16) NOT NULL DEFAULT 'wiki',
    anchor          VARCHAR(256) NOT NULL DEFAULT '',
    create_time     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_wiki_link_from ON t_wiki_link (from_page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_link_to ON t_wiki_link (to_page_id);
