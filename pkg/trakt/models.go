package trakt

import "time"

// DeviceCodeResponse is returned when requesting a device code for pairing.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse represents OAuth2 tokens returned by Trakt.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	CreatedAt    int64  `json:"created_at"`
}

// IDs contains external database and Trakt identifiers for media items.
type IDs struct {
	Trakt int    `json:"trakt"`
	Slug  string `json:"slug,omitempty"`
	TVDB  int    `json:"tvdb,omitempty"`
	IMDB  string `json:"imdb,omitempty"`
	TMDB  int    `json:"tmdb,omitempty"`
}

// Movie represents movie metadata from Trakt.
type Movie struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
	IDs   IDs    `json:"ids"`
}

// Show represents TV show metadata from Trakt.
type Show struct {
	Title string `json:"title"`
	Year  int    `json:"year,omitempty"`
	IDs   IDs    `json:"ids"`
}

// Season represents a TV season.
type Season struct {
	Number   int       `json:"number"`
	Episodes []Episode `json:"episodes,omitempty"`
}

// Episode represents a TV show episode.
type Episode struct {
	Season int    `json:"season"`
	Number int    `json:"number"`
	Title  string `json:"title,omitempty"`
	IDs    IDs    `json:"ids,omitempty"`
}

// WatchedMovie represents a movie marked as watched on Trakt.
type WatchedMovie struct {
	Plays         int       `json:"plays"`
	LastWatchedAt time.Time `json:"last_watched_at"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
	Movie         Movie     `json:"movie"`
}

// WatchedShow represents a TV show marked as watched on Trakt.
type WatchedShow struct {
	Plays         int             `json:"plays"`
	LastWatchedAt time.Time       `json:"last_watched_at"`
	LastUpdatedAt time.Time       `json:"last_updated_at"`
	ResetAt       *time.Time      `json:"reset_at,omitempty"`
	Show          Show            `json:"show"`
	Seasons       []WatchedSeason `json:"seasons"`
}

// WatchedSeason represents a watched season of a TV show.
type WatchedSeason struct {
	Number   int              `json:"number"`
	Episodes []WatchedEpisode `json:"episodes"`
}

// WatchedEpisode represents an episode that has been watched.
type WatchedEpisode struct {
	Number        int       `json:"number"`
	Plays         int       `json:"plays"`
	LastWatchedAt time.Time `json:"last_watched_at"`
}

// PlaybackItem represents an active in-progress media playback on Trakt.
type PlaybackItem struct {
	ID       int64     `json:"id"`
	Progress float64   `json:"progress"`
	PausedAt time.Time `json:"paused_at"`
	Type     string    `json:"type"` // "movie" or "episode"
	Movie    *Movie    `json:"movie,omitempty"`
	Episode  *Episode  `json:"episode,omitempty"`
	Show     *Show     `json:"show,omitempty"`
}

// ScrobbleRequest is the payload sent to start, pause, or stop scrobbling.
type ScrobbleRequest struct {
	Movie      *Movie   `json:"movie,omitempty"`
	Show       *Show    `json:"show,omitempty"`
	Episode    *Episode `json:"episode,omitempty"`
	Progress   float64  `json:"progress"`
	AppVersion string   `json:"app_version,omitempty"`
	AppDate    string   `json:"app_date,omitempty"`
}

// ScrobbleResponse is returned after a scrobble action.
type ScrobbleResponse struct {
	ID       int64           `json:"id"`
	Action   string          `json:"action"` // "start", "pause", "scrobble"
	Progress float64         `json:"progress"`
	Sharing  map[string]bool `json:"sharing,omitempty"`
	Movie    *Movie          `json:"movie,omitempty"`
	Episode  *Episode        `json:"episode,omitempty"`
	Show     *Show           `json:"show,omitempty"`
}
