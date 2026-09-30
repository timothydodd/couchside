// Package livetv adds live TV and DVR on top of an HDHomeRun network tuner:
// channel lineup, program guide, live HLS streams and scheduled recordings.
package livetv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HDHomeRun talks to one tuner over its local HTTP API.
type HDHomeRun struct {
	base   string // e.g. http://192.168.1.50
	client *http.Client
}

// NewHDHomeRun accepts an IP, host name or URL.
func NewHDHomeRun(addr string) *HDHomeRun {
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return &HDHomeRun{base: addr, client: &http.Client{Timeout: 10 * time.Second}}
}

type Device struct {
	FriendlyName    string `json:"FriendlyName"`
	ModelNumber     string `json:"ModelNumber"`
	DeviceID        string `json:"DeviceID"`
	DeviceAuth      string `json:"DeviceAuth"`
	FirmwareVersion string `json:"FirmwareVersion"`
	TunerCount      int    `json:"TunerCount"`
	LineupURL       string `json:"LineupURL"`
}

type LineupEntry struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	VideoCodec  string `json:"VideoCodec"`
	AudioCodec  string `json:"AudioCodec"`
	HD          int    `json:"HD"`
	DRM         int    `json:"DRM"`
	URL         string `json:"URL"`
	// Readings from the tuner's last channel scan; absent for some channels.
	SignalStrength *int `json:"SignalStrength"`
	SignalQuality  *int `json:"SignalQuality"`
}

type TunerStatus struct {
	Resource  string `json:"Resource"`
	VctNumber string `json:"VctNumber"`
	VctName   string `json:"VctName"`
	TargetIP  string `json:"TargetIP"`
}

func (h *HDHomeRun) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("hdhomerun: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hdhomerun: %s: HTTP %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

func (h *HDHomeRun) Discover(ctx context.Context) (*Device, error) {
	var d Device
	if err := h.getJSON(ctx, h.base+"/discover.json", &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (h *HDHomeRun) Lineup(ctx context.Context) ([]LineupEntry, error) {
	var l []LineupEntry
	err := h.getJSON(ctx, h.base+"/lineup.json", &l)
	return l, err
}

// Status lists tuners; an entry with VctNumber set is in use.
func (h *HDHomeRun) Status(ctx context.Context) ([]TunerStatus, error) {
	var s []TunerStatus
	err := h.getJSON(ctx, h.base+"/status.json", &s)
	return s, err
}

// --- SiliconDust guide service -----------------------------------------------------

type guideChannel struct {
	GuideNumber string       `json:"GuideNumber"`
	GuideName   string       `json:"GuideName"`
	Affiliate   string       `json:"Affiliate"`
	ImageURL    string       `json:"ImageURL"`
	Guide       []guideEntry `json:"Guide"`
}

type guideEntry struct {
	StartTime       int64    `json:"StartTime"`
	EndTime         int64    `json:"EndTime"`
	First           int      `json:"First"` // 1 = first airing ("new")
	Title           string   `json:"Title"`
	EpisodeNumber   string   `json:"EpisodeNumber"`
	EpisodeTitle    string   `json:"EpisodeTitle"`
	Synopsis        string   `json:"Synopsis"`
	OriginalAirdate int64    `json:"OriginalAirdate"`
	SeriesID        string   `json:"SeriesID"`
	ImageURL        string   `json:"ImageURL"`
	Filter          []string `json:"Filter"`
}

var guideClient = &http.Client{Timeout: 30 * time.Second}

// fetchGuide returns roughly four hours of listings starting at start (0 = now).
// DeviceAuth rotates, so it's read fresh from the tuner before each call.
func fetchGuide(ctx context.Context, deviceAuth string, start int64) ([]guideChannel, error) {
	q := url.Values{"DeviceAuth": {deviceAuth}}
	if start > 0 {
		q.Set("Start", strconv.FormatInt(start, 10))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.hdhomerun.com/api/guide.php?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// The guide service rejects Go's default User-Agent with 403.
	req.Header.Set("User-Agent", "Couchside (+https://github.com/timothydodd/couchside)")
	resp, err := guideClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("guide: %s", strings.ReplaceAll(err.Error(), deviceAuth, "***"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("guide: HTTP %d", resp.StatusCode)
	}
	var out []guideChannel
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("guide: %w", err)
	}
	return out, nil
}

// channelSortKey orders "2.1" < "2.10" < "11.1" numerically.
func channelSortKey(n string) float64 {
	major, minor, _ := strings.Cut(n, ".")
	a, _ := strconv.Atoi(major)
	b, _ := strconv.Atoi(minor)
	return float64(a) + float64(b)/1000
}
