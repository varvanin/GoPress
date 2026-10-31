package store

import (
	"encoding/json"
	"path"
	"strconv"
	"strings"
)

type ImageVariant struct{ Size, Width, Height int }
type ImageInfo struct {
	Width, Height int
	Variants      []ImageVariant
}

func ImageVariantName(name string, size int) string {
	ext := path.Ext(name)
	return strings.TrimSuffix(name, ext) + "-" + strconv.Itoa(size) + ext
}
func (s *Store) ImageInfos(names []string) (map[string]ImageInfo, error) {
	result := map[string]ImageInfo{}
	if len(names) == 0 {
		return result, nil
	}
	raw, err := json.Marshal(names)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(`SELECT name,width,height,variants FROM media_images WHERE name IN(SELECT value FROM json_each(?))`, string(raw))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, data string
		var info ImageInfo
		if err = rows.Scan(&name, &info.Width, &info.Height, &data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(data), &info.Variants); err != nil {
			return nil, err
		}
		result[name] = info
	}
	return result, rows.Err()
}
