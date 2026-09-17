-- Reverse 000032 as far as the data allows.
--
-- This down migration is deliberately partial, for the same reason 000029's and
-- 000025's are: the up migration deleted a link row, and which classification
-- carried the tag is not recoverable from the surviving schema alone. Restoring
-- the taxonomy row gives back the vocabulary; it does not, by itself, re-tag the
-- posting.
--
-- What IS recoverable: the up migration's comments record the exact link this
-- removed -- classification 6255, job_posting_id 45165 -- so this down migration
-- restores both the taxonomy row and that one link row, not just the vocabulary.
-- That is the one link known to exist at authoring time; if the posting has since
-- been re-classified, re-attaching this tag to classification 6255 may no longer
-- reflect the posting's current classification, and the right move is to review
-- and re-classify rather than trust this restore blindly.

INSERT INTO specializations (slug, name) VALUES
    ('japan-market', 'Japan Markets')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO job_posting_specializations (classification_id, specialization_id)
SELECT 6255, s.id FROM specializations s WHERE s.slug = 'japan-market'
ON CONFLICT DO NOTHING;
