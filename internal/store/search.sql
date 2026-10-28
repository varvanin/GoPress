CREATE VIRTUAL TABLE search_index USING fts5(title,body,slug UNINDEXED,tokenize='unicode61 remove_diacritics 2');
CREATE TRIGGER search_pages_insert AFTER INSERT ON pages WHEN NEW.published_slug IS NOT NULL AND NEW.trashed_at=0 BEGIN
 INSERT INTO search_index(rowid,title,body,slug) VALUES(NEW.id,NEW.published_title,(SELECT coalesce(group_concat(value,' '),'') FROM json_tree(NEW.published_document) WHERE key='text' AND type='text'),NEW.published_slug);
END;
CREATE TRIGGER search_pages_update AFTER UPDATE OF published_slug,published_title,published_document,trashed_at ON pages BEGIN
 DELETE FROM search_index WHERE rowid=OLD.id;
 INSERT INTO search_index(rowid,title,body,slug) SELECT NEW.id,NEW.published_title,(SELECT coalesce(group_concat(value,' '),'') FROM json_tree(NEW.published_document) WHERE key='text' AND type='text'),NEW.published_slug WHERE NEW.published_slug IS NOT NULL AND NEW.trashed_at=0;
END;
CREATE TRIGGER search_pages_delete AFTER DELETE ON pages BEGIN DELETE FROM search_index WHERE rowid=OLD.id; END;
INSERT INTO search_index(rowid,title,body,slug) SELECT id,published_title,(SELECT coalesce(group_concat(value,' '),'') FROM json_tree(pages.published_document) WHERE key='text' AND type='text'),published_slug FROM pages WHERE published_slug IS NOT NULL AND trashed_at=0;
PRAGMA user_version=10;
