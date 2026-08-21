package zimfs

type ArchiveInfo struct {
	Filename     string `json:"filename"`
	Title        string `json:"title"`
	Date         string `json:"date"`
	Language     string `json:"language"`
	Source       string `json:"source"`
	Description  string `json:"description"`
	EntryCount   uint32 `json:"entry_count"`
	ArticleCount uint32 `json:"article_count"`
	MediaCount   uint32 `json:"media_count"`
	SizeBytes    int64  `json:"size_bytes"`
	Checksum     string `json:"checksum"`
}

