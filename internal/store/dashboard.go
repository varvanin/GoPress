package store

import "strings"

type DashboardData struct {
	Pages, Posts           PageSummary
	Scheduled, Trashed     int
	MediaCount, MediaBytes int64
	Recent, Upcoming       []Page
}

func (s *Store) Dashboard() (DashboardData, error) {
	var d DashboardData
	err := s.DB.QueryRow(`SELECT
 count(CASE WHEN kind='page' AND trashed_at=0 THEN 1 END),count(CASE WHEN kind='page' AND trashed_at=0 AND published_slug IS NOT NULL THEN 1 END),count(CASE WHEN kind='page' AND trashed_at=0 AND published_slug IS NULL THEN 1 END),
 count(CASE WHEN kind='post' AND trashed_at=0 THEN 1 END),count(CASE WHEN kind='post' AND trashed_at=0 AND published_slug IS NOT NULL THEN 1 END),count(CASE WHEN kind='post' AND trashed_at=0 AND published_slug IS NULL THEN 1 END),
 count(CASE WHEN scheduled_at>0 AND trashed_at=0 THEN 1 END),count(CASE WHEN trashed_at>0 THEN 1 END) FROM pages`).Scan(&d.Pages.Total, &d.Pages.Published, &d.Pages.Drafts, &d.Posts.Total, &d.Posts.Published, &d.Posts.Drafts, &d.Scheduled, &d.Trashed)
	if err != nil {
		return d, err
	}
	if err = s.DB.QueryRow("SELECT count(*),coalesce(sum(size),0) FROM media").Scan(&d.MediaCount, &d.MediaBytes); err != nil {
		return d, err
	}
	// Only these fixed queries use the order clause; documents stay out of the overview.
	columns := strings.Replace(pageColumns, ",document,", ",'' AS document,", 1)
	columns = strings.Replace(columns, ",translations", ",'{}' AS translations", 1)
	read := func(columns, where, order string, limit int) ([]Page, error) {
		rows, e := s.DB.Query("SELECT "+columns+" FROM pages WHERE "+where+" ORDER BY "+order+" LIMIT ?", limit)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		result := []Page{}
		for rows.Next() {
			p, e := scanPage(rows)
			if e != nil {
				return nil, e
			}
			result = append(result, p)
		}
		return result, rows.Err()
	}
	d.Recent, err = read(columns, "trashed_at=0", "updated_at DESC,id DESC", 8)
	if err != nil {
		return d, err
	}
	pinnedColumns := strings.Replace(columns, ",title,", ",coalesce((SELECT title FROM revisions WHERE page_id=pages.id AND version=pages.scheduled_version),title) AS title,", 1)
	d.Upcoming, err = read(pinnedColumns, "trashed_at=0 AND scheduled_at>0", "scheduled_at,id", 3)
	return d, err
}
