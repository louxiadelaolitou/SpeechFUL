-- +goose Up
UPDATE public.channels AS c
SET name = labels.name, updated_at = now()
FROM (VALUES
    ('vent', '校园吐槽'),
    ('warning', '避雷指南'),
    ('recommendation', '吐血推荐'),
    ('buddy', '搭子寻觅'),
    ('emotion', '情感树洞'),
    ('mutual_help', '互助问答'),
    ('technology', '人工智能')
) AS labels(code, name)
WHERE c.code = labels.code;

INSERT INTO public.channels (id, code, name, initially_visible, display_order)
VALUES
    ('5f6f4f88-8f64-4bb2-9d34-000000000008', 'clubs', '社团活动', false, 80),
    ('5f6f4f88-8f64-4bb2-9d34-000000000009', 'careers', '求职经验', false, 90),
    ('5f6f4f88-8f64-4bb2-9d34-000000000010', 'alumni', '校友评论', false, 100);

INSERT INTO c_posts.ranking_inputs (channel_id)
SELECT id FROM public.channels WHERE code IN ('clubs', 'careers', 'alumni');

ALTER TABLE c_posts.feed_preferences
    DROP CONSTRAINT feed_preferences_blocked_channel_ids_check,
    ADD CONSTRAINT feed_preferences_blocked_channel_ids_check
        CHECK (cardinality(blocked_channel_ids) <= 10);
ALTER TABLE c_posts.ranking_snapshots
    DROP CONSTRAINT ranking_snapshots_channel_ids_check,
    ADD CONSTRAINT ranking_snapshots_channel_ids_check
        CHECK (cardinality(channel_ids) BETWEEN 1 AND 10);
ALTER TABLE c_posts.hot_snapshots
    DROP CONSTRAINT hot_snapshots_channel_ids_check,
    ADD CONSTRAINT hot_snapshots_channel_ids_check
        CHECK (cardinality(channel_ids) BETWEEN 1 AND 10);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Campus categories may contain posts and preferences; freeze and forward repair instead';
END $$;
-- +goose StatementEnd
