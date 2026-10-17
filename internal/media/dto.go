package media

type UploadForm struct {
	Purpose string `json:"purpose" form:"purpose" binding:"required,oneof=listing avatar merchant review complaint"`
}

type MediaDTO struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"` // image | video
	URL        string  `json:"url"`
	ThumbURL   *string `json:"thumb_url"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	DurationMs *int    `json:"duration_ms,omitempty"` // hanya video
}
