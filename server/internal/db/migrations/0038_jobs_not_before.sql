-- A job that failed for a reason that passes (the NAS dropped, TMDB rate
-- limited) is queued again for later instead of failing: ClaimJob leaves it
-- alone until not_before.
ALTER TABLE jobs ADD COLUMN not_before INTEGER;
