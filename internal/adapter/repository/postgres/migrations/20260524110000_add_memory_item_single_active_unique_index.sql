DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM t_memory_item
        WHERE deleted = 0
          AND status = 'active'
          AND canonical_key IN (
              'response.language',
              'workflow.first_step',
              'project.constraint.network',
              'project.messaging.main_bus'
          )
        GROUP BY user_id, scope_type, COALESCE(scope_id, ''), canonical_key
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot create unique index uk_memory_item_single_active: duplicate active single-valued memory items exist';
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uk_memory_item_single_active
    ON t_memory_item (user_id, scope_type, COALESCE(scope_id, ''), canonical_key)
    WHERE deleted = 0
      AND status = 'active'
      AND canonical_key IN (
          'response.language',
          'workflow.first_step',
          'project.constraint.network',
          'project.messaging.main_bus'
      );
