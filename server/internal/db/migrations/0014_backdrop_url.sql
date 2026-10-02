-- The provider's backdrop image (TMDB has them; OMDb doesn't). When it's
-- empty, the artwork job grabs a frame from the video instead.
ALTER TABLE media_items ADD COLUMN backdrop_url TEXT NOT NULL DEFAULT '';
