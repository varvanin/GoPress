ALTER TABLE pages ADD COLUMN seo TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(seo));
ALTER TABLE pages ADD COLUMN published_seo TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(published_seo));
ALTER TABLE pages ADD COLUMN published_updated_at INTEGER NOT NULL DEFAULT 0;
UPDATE pages SET published_updated_at=published_at WHERE published_slug IS NOT NULL;
ALTER TABLE revisions ADD COLUMN seo TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(seo));
CREATE INDEX pages_sitemap ON pages(id,published_slug,published_updated_at)
 WHERE trashed_at=0 AND published_slug IS NOT NULL AND coalesce(json_extract(published_seo,'$.noindex'),0)=0;
CREATE TABLE appearance(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL DEFAULT 0,value TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(value)));
INSERT INTO appearance(id) VALUES(1);
PRAGMA user_version=6;
