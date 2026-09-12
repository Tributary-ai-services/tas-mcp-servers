package napkin

// SubmitRequest is the request body for visual generation
type SubmitRequest struct {
	Content    string `json:"content"`
	Format     string `json:"format,omitempty"`
	StyleId    string `json:"style_id,omitempty"`
	ColorMode  string `json:"color_mode,omitempty"`
	Language   string `json:"language,omitempty"`
	Variations int    `json:"variations,omitempty"`
	Context    string `json:"context,omitempty"`
}

// SubmitResponse is the response from visual submission
type SubmitResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// StatusResponse is the response from status polling
type StatusResponse struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Progress    int        `json:"progress,omitempty"`
	Files       []FileInfo `json:"files,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   string     `json:"created_at"`
	CompletedAt string     `json:"completed_at,omitempty"`
}

// FileInfo describes a generated file
type FileInfo struct {
	Index     int    `json:"index"`
	Format    string `json:"format"`
	ColorMode string `json:"color_mode"`
	URL       string `json:"url"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}
