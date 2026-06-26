package geoip

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

const (
	defaultFilename = "country.mmdb"
	Source          = "MetaCubeX/meta-rules-dat latest country.mmdb"
)

type Service struct {
	dataDir string
	db      *sql.DB
	client  *http.Client
	mu      sync.RWMutex
	reader  *maxminddb.Reader
	path    string
}

func NewService(dataDir string, db *sql.DB) *Service {
	return &Service{
		dataDir: dataDir,
		db:      db,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Service) LoadExisting() {
	if s == nil {
		return
	}
	path := filepath.Join(s.dataDir, defaultFilename)
	if err := s.loadFile(path); err != nil {
		s.storeStatus("", 0, 0, err.Error(), "")
	}
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reader != nil {
		_ = s.reader.Close()
		s.reader = nil
	}
}

func (s *Service) LookupCountry(ipText string) string {
	if s == nil || strings.TrimSpace(ipText) == "" {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(ipText))
	if ip == nil {
		return ""
	}
	s.mu.RLock()
	reader := s.reader
	s.mu.RUnlock()
	if reader == nil {
		return ""
	}
	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		RegisteredCountry struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"registered_country"`
	}
	if err := reader.Lookup(ip, &record); err != nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(firstNonEmpty(record.Country.ISOCode, record.RegisteredCountry.ISOCode)))
}

func (s *Service) UpdateFromMetaCubeXLatest() error {
	if s == nil {
		return errors.New("geoip service not initialized")
	}
	assetURL, checksum, err := s.latestMetaCubeXCountryMMDBAsset()
	if err != nil {
		s.storeStatus("", 0, time.Now().UnixMilli(), err.Error(), "")
		return err
	}
	path, sha, err := s.downloadAndReplace(assetURL, checksum)
	if err != nil {
		s.storeStatus("", 0, time.Now().UnixMilli(), err.Error(), "")
		return err
	}
	now := time.Now().UnixMilli()
	s.storeStatus(path, now, now, "", sha)
	return nil
}

func (s *Service) loadFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("geoip path is empty")
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.reader != nil {
		_ = s.reader.Close()
	}
	s.reader = reader
	s.path = path
	s.mu.Unlock()
	s.storeStatus(path, time.Now().UnixMilli(), 0, "", fileSHA256(path))
	return nil
}

func (s *Service) latestMetaCubeXCountryMMDBAsset() (string, string, error) {
	resp, err := s.httpGet("https://api.github.com/repos/MetaCubeX/meta-rules-dat/releases/latest")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", errors.New("fetch geoip release returned " + resp.Status)
	}
	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&release); err != nil {
		return "", "", err
	}
	assetURL := ""
	checksumURL := ""
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if name == "country.mmdb" {
			assetURL = asset.BrowserDownloadURL
		}
		if strings.Contains(name, "country.mmdb") && (strings.Contains(name, "sha256") || strings.HasSuffix(name, ".sha256sum")) {
			checksumURL = asset.BrowserDownloadURL
		}
	}
	if assetURL == "" {
		return "", "", errors.New("country.mmdb asset not found")
	}
	checksum := ""
	if checksumURL != "" {
		checksum = s.fetchSHA256Text(checksumURL)
	}
	return assetURL, checksum, nil
}

func (s *Service) fetchSHA256Text(url string) string {
	resp, err := s.httpGet(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return ""
	}
	for _, field := range strings.Fields(string(raw)) {
		if len(field) == 64 {
			return strings.ToLower(field)
		}
	}
	return ""
}

func (s *Service) downloadAndReplace(assetURL, expectedSHA256 string) (string, string, error) {
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return "", "", err
	}
	resp, err := s.httpGet(assetURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", errors.New("download geoip returned " + resp.Status)
	}
	tmp, err := os.CreateTemp(s.dataDir, ".country-*.mmdb")
	if err != nil {
		return "", "", err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	hasher := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(io.LimitReader(resp.Body, 128*1024*1024), hasher)); err != nil {
		_ = tmp.Close()
		return "", "", err
	}
	if err := tmp.Close(); err != nil {
		return "", "", err
	}
	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA256 != "" && !strings.EqualFold(expectedSHA256, actualSHA) {
		return "", "", errors.New("geoip checksum mismatch")
	}
	if reader, err := maxminddb.Open(tmpPath); err != nil {
		return "", "", err
	} else {
		_ = reader.Close()
	}
	finalPath := filepath.Join(s.dataDir, defaultFilename)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", "", err
	}
	if err := s.loadFile(finalPath); err != nil {
		return "", "", err
	}
	return finalPath, actualSHA, nil
}

func (s *Service) storeStatus(path string, loadedAt, updatedAt int64, errorText, sha string) {
	if s == nil || s.db == nil {
		return
	}
	if path == "" {
		path = s.path
	}
	if sha == "" && path != "" {
		sha = fileSHA256(path)
	}
	_, _ = s.db.Exec(
		`INSERT INTO geoip_status (id, file_path, loaded_at, updated_at, last_error, sha256)
		 VALUES (1, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			file_path = CASE WHEN excluded.file_path != '' THEN excluded.file_path ELSE geoip_status.file_path END,
			loaded_at = CASE WHEN excluded.loaded_at != 0 THEN excluded.loaded_at ELSE geoip_status.loaded_at END,
			updated_at = CASE WHEN excluded.updated_at != 0 THEN excluded.updated_at ELSE geoip_status.updated_at END,
			last_error = excluded.last_error,
			sha256 = CASE WHEN excluded.sha256 != '' THEN excluded.sha256 ELSE geoip_status.sha256 END`,
		path,
		loadedAt,
		updatedAt,
		errorText,
		sha,
	)
}

func (s *Service) httpGet(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req)
}

func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return ""
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
