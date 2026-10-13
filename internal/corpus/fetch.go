package corpus

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// FetchOptions configures remote source downloading.
type FetchOptions struct {
	// CacheDir is where downloaded sources are cached. Defaults to
	// <temp>/gaslit-datatest-cache.
	CacheDir string
	// Force re-downloads even when a cached copy exists.
	Force bool
	// Client is the HTTP client to use. Defaults to a 2-minute-timeout client.
	Client *http.Client
	// MaxBytes caps the download size. Default 64 MiB.
	MaxBytes int64
}

// DefaultCacheDir returns the default download cache directory.
func DefaultCacheDir() string {
	return filepath.Join(os.TempDir(), "gaslit-datatest-cache")
}

func (o FetchOptions) resolve() FetchOptions {
	if o.CacheDir == "" {
		o.CacheDir = DefaultCacheDir()
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 2 * time.Minute}
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 64 << 20
	}
	return o
}

// IsURL reports whether s is an http(s) URL.
func IsURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Fetch downloads rawURL into the cache directory and returns the local path.
// When a cached copy already exists (and Force is false) the download is
// skipped, so repeated runs reuse the same file.
func Fetch(ctx context.Context, rawURL string, opt FetchOptions) (string, error) {
	opt = opt.resolve()
	if !IsURL(rawURL) {
		return "", fmt.Errorf("corpus: not an http(s) URL: %q", rawURL)
	}
	if err := os.MkdirAll(opt.CacheDir, 0o755); err != nil {
		return "", fmt.Errorf("corpus: create cache dir: %w", err)
	}

	dest := cachePath(opt.CacheDir, rawURL)
	if !opt.Force {
		if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
			return dest, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gaslit-datatest/1.0 (+https://github.com/ItsArnavSh/gaslit)")

	resp, err := opt.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("corpus: download %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("corpus: download %s: unexpected status %s", rawURL, resp.Status)
	}

	tmp, err := os.CreateTemp(opt.CacheDir, "dl-*.part")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	limited := io.LimitReader(resp.Body, opt.MaxBytes+1)
	n, err := io.Copy(tmp, limited)
	if err != nil {
		return "", fmt.Errorf("corpus: download %s: %w", rawURL, err)
	}
	if n > opt.MaxBytes {
		return "", fmt.Errorf("corpus: download %s exceeds %d bytes", rawURL, opt.MaxBytes)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// cachePath builds a stable, collision-resistant cache filename for a URL.
func cachePath(dir, rawURL string) string {
	sum := sha1.Sum([]byte(rawURL))
	hash := hex.EncodeToString(sum[:])[:10]

	base := "download"
	if u, err := url.Parse(rawURL); err == nil {
		if b := path.Base(u.Path); b != "" && b != "/" && b != "." {
			base = b
		}
	}
	return filepath.Join(dir, hash+"-"+sanitizeName(base))
}

// sanitizeName keeps a filename safe for the filesystem while preserving its
// extension.
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "download"
	}
	return out
}
