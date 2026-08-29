package torbox

import (
	"encoding/json"
	"time"
)

// APIResponse represents the standard TorBox API JSON envelope.
type APIResponse[T any] struct {
	Success bool    `json:"success"`
	Error   *string `json:"error,omitempty"`
	Detail  string  `json:"detail,omitempty"`
	Data    T       `json:"data"`
}

// User represents a TorBox user account profile.
type User struct {
	ID              int       `json:"id"`
	Email           string    `json:"email"`
	Plan            int       `json:"plan"`
	TotalDownloaded int64     `json:"total_downloaded,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	AuthID          string    `json:"auth_id,omitempty"`
}

// Torrent represents a torrent item in TorBox.
type Torrent struct {
	ID            int           `json:"id"`
	Hash          string        `json:"hash"`
	Name          string        `json:"name"`
	Size          int64         `json:"size"`
	Active        bool          `json:"active"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	DownloadState string        `json:"download_state"`
	DownloadSpeed int64         `json:"download_speed"`
	UploadSpeed   int64         `json:"upload_speed"`
	Progress      float64       `json:"progress"`
	ETA           int64         `json:"eta"`
	Seeds         int           `json:"seeds"`
	Peers         int           `json:"peers"`
	Ratio         float64       `json:"ratio"`
	Cached        bool          `json:"cached"`
	Files         []TorrentFile `json:"files"`
}

// TorrentFile represents a file inside a torrent.
type TorrentFile struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	MimeType     string `json:"mimetype,omitempty"`
	ShortName    string `json:"short_name,omitempty"`
	AbsolutePath string `json:"absolute_path,omitempty"`
	S3Path       string `json:"s3_path,omitempty"`
}

// UsenetItem represents a Usenet download item in TorBox.
type UsenetItem struct {
	ID            int          `json:"id"`
	Hash          string       `json:"hash,omitempty"`
	Name          string       `json:"name"`
	Size          int64        `json:"size"`
	Active        bool         `json:"active"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	DownloadState string       `json:"download_state"`
	DownloadSpeed int64        `json:"download_speed"`
	Progress      float64      `json:"progress"`
	ETA           int64        `json:"eta"`
	Files         []UsenetFile `json:"files"`
}

// UsenetFile represents a file inside a Usenet download.
type UsenetFile struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	MimeType     string `json:"mimetype,omitempty"`
	ShortName    string `json:"short_name,omitempty"`
	AbsolutePath string `json:"absolute_path,omitempty"`
	S3Path       string `json:"s3_path,omitempty"`
}

// WebDLItem represents a Web Download item in TorBox.
type WebDLItem struct {
	ID            int         `json:"id"`
	Hash          string      `json:"hash,omitempty"`
	Name          string      `json:"name"`
	Size          int64       `json:"size"`
	Active        bool        `json:"active"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	DownloadState string      `json:"download_state"`
	DownloadSpeed int64       `json:"download_speed"`
	Progress      float64     `json:"progress"`
	ETA           int64       `json:"eta,omitempty"`
	Files         []WebDLFile `json:"files"`
}

// WebDLFile represents a file inside a Web download item.
type WebDLFile struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	MimeType     string `json:"mimetype,omitempty"`
	ShortName    string `json:"short_name,omitempty"`
	AbsolutePath string `json:"absolute_path,omitempty"`
	S3Path       string `json:"s3_path,omitempty"`
}

// CreateTorrentRequest contains the parameters for adding a torrent.
type CreateTorrentRequest struct {
	Magnet      string `json:"magnet,omitempty"`
	TorrentFile []byte `json:"-"`
	FileName    string `json:"-"`
	Seed        int    `json:"seed,omitempty"`
	AllowZip    bool   `json:"allow_zip,omitempty"`
	Name        string `json:"name,omitempty"`
}

// CreateTorrentResponse is the data returned when creating a torrent.
type CreateTorrentResponse struct {
	TorrentID int    `json:"torrent_id"`
	Hash      string `json:"hash"`
	AuthID    string `json:"auth_id,omitempty"`
}

// ControlTorrentRequest contains parameters for controlling a torrent.
type ControlTorrentRequest struct {
	TorrentID int    `json:"torrent_id"`
	Operation string `json:"operation"` // "delete", "pause", "resume", "reannounce"
	All       bool   `json:"all,omitempty"`
}

// CreateUsenetRequest contains parameters for adding a Usenet download.
type CreateUsenetRequest struct {
	Link     string `json:"link,omitempty"`
	NZBFile  []byte `json:"-"`
	FileName string `json:"-"`
	Name     string `json:"name,omitempty"`
}

// CreateUsenetResponse is the data returned when creating a usenet download.
type CreateUsenetResponse struct {
	UsenetID int    `json:"usenet_id"`
	Hash     string `json:"hash,omitempty"`
}

// ControlUsenetRequest contains parameters for controlling a Usenet download.
type ControlUsenetRequest struct {
	UsenetID  int    `json:"usenet_id"`
	Operation string `json:"operation"` // "delete", "pause", "resume", "reannounce"
	All       bool   `json:"all,omitempty"`
}

// CreateWebDLRequest contains parameters for adding a Web download.
type CreateWebDLRequest struct {
	Link string `json:"link"`
	Name string `json:"name,omitempty"`
}

// CreateWebDLResponse is the data returned when creating a web download.
type CreateWebDLResponse struct {
	WebDLID int    `json:"webdownload_id"`
	Hash    string `json:"hash,omitempty"`
}

// ControlWebDLRequest contains parameters for controlling a Web download.
type ControlWebDLRequest struct {
	WebDLID   int    `json:"webdl_id"`
	Operation string `json:"operation"` // "delete", "pause", "resume"
	All       bool   `json:"all,omitempty"`
}

// DownloadLink represents resolved download or streaming URL.
type DownloadLink struct {
	URL string `json:"url"`
}

// UnmarshalJSON handles both raw string and object with "url" field.
func (d *DownloadLink) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		d.URL = str
		return nil
	}
	type alias DownloadLink
	var obj alias
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	d.URL = obj.URL
	return nil
}
