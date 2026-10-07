-- Migrate subscriptions written before the hierarchical topic catalog.
WITH legacy AS (
    SELECT s.user_id,
           COALESCE((
               SELECT jsonb_agg(topic ORDER BY first_position)
               FROM (
                   SELECT topic, MIN(position) AS first_position
                   FROM (
                       SELECT CASE value
                                  WHEN 'ai-models' THEN 'tech.ai.models'
                                  WHEN 'ai-research' THEN 'tech.ai.research'
                                  WHEN 'ai-tools' THEN 'tech.ai.tools'
                                  WHEN 'developer-trends' THEN 'tech.dev'
                                  WHEN 'startups-and-industry' THEN 'tech.startups'
                                  ELSE value
                              END AS topic, position
                       FROM jsonb_array_elements_text(s.topics_json) WITH ORDINALITY AS original(value, position)
                   ) translated
                   GROUP BY topic
               ) unique_topics
           ), '[]'::jsonb) AS topics_json
    FROM t_daily_brief_subscription s
    WHERE s.topics_json ?| ARRAY['ai-models', 'ai-research', 'ai-tools', 'developer-trends', 'startups-and-industry']
), tech_sources(topic, source) AS (
    VALUES
        ('tech.ai.models', 'openai-blog'),
        ('tech.ai.models', 'anthropic-blog'),
        ('tech.ai.models', 'google-deepmind-blog'),
        ('tech.ai.models', 'meta-ai-blog'),
        ('tech.ai.research', 'arxiv-cs-ai'),
        ('tech.ai.research', 'arxiv-cs-cl'),
        ('tech.ai.research', 'arxiv-cs-lg'),
        ('tech.ai.tools', 'papers-with-code'),
        ('tech.dev', 'hacker-news'),
        ('tech.dev', 'github-trending'),
        ('tech.startups', 'the-decoder'),
        ('tech.startups', 'venturebeat-ai'),
        ('tech.startups', 'techcrunch-ai')
), migrated AS (
    SELECT legacy.user_id, legacy.topics_json,
           COALESCE((
               SELECT jsonb_agg(source ORDER BY source)
               FROM (
                   SELECT DISTINCT source
                   FROM (
                       SELECT value AS source
                       FROM jsonb_array_elements_text(s.sources_json) AS existing(value)
                       WHERE value NOT IN (SELECT tech_sources.source FROM tech_sources)
                       UNION ALL
                       SELECT tech_sources.source
                       FROM tech_sources
                       WHERE legacy.topics_json ? tech_sources.topic
                   ) combined
               ) unique_sources
           ), '[]'::jsonb) AS sources_json
    FROM legacy
    JOIN t_daily_brief_subscription s ON s.user_id = legacy.user_id
)
UPDATE t_daily_brief_subscription AS subscription
SET topics_json = migrated.topics_json,
    sources_json = migrated.sources_json,
    update_time = CURRENT_TIMESTAMP
FROM migrated
WHERE subscription.user_id = migrated.user_id;
