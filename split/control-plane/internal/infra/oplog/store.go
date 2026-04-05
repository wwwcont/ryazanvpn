package oplog

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Record struct {
	Timestamp   time.Time `json:"ts"`
	Service     string    `json:"service"`
	Severity    string    `json:"severity"`
	ErrorCode   string    `json:"error_code,omitempty"`
	RequestID   string    `json:"request_id,omitempty"`
	NodeID      string    `json:"node_id,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	DeviceID    string    `json:"device_id,omitempty"`
	OperationID string    `json:"operation_id,omitempty"`
	Message     string    `json:"message,omitempty"`
}

type Filter struct {
	From    time.Time
	To      time.Time
	Service string
	NodeID  string
	Level   string
	Limit   int
}

type Store struct {
	dir       string
	retention time.Duration
	mu        sync.Mutex
}

func NewStore(dir string, retention time.Duration) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("log dir is required")
	}
	if retention <= 0 {
		retention = 7 * 24 * time.Hour
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir, retention: retention}, nil
}

func (s *Store) Write(_ context.Context, rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	if strings.TrimSpace(rec.Severity) == "" {
		rec.Severity = "info"
	}
	if err := s.compressAndCleanupLocked(time.Now().UTC()); err != nil {
		return err
	}
	name := filepath.Join(s.dir, "ops-"+rec.Timestamp.UTC().Format("20060102")+".ndjson")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(rec)
}

func (s *Store) Export(ctx context.Context, w io.Writer, flt Filter) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if flt.Limit <= 0 || flt.Limit > 10000 {
		flt.Limit = 5000
	}
	files, err := filepath.Glob(filepath.Join(s.dir, "ops-*.ndjson*"))
	if err != nil {
		return 0, err
	}
	sort.Strings(files)
	written := 0
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		r, closeFn, err := openMaybeGzip(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			var rec Record
			if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
				continue
			}
			if !match(rec, flt) {
				continue
			}
			if err := json.NewEncoder(w).Encode(rec); err != nil {
				closeFn()
				return written, err
			}
			written++
			if written >= flt.Limit {
				closeFn()
				return written, nil
			}
		}
		closeFn()
	}
	return written, nil
}

func match(rec Record, flt Filter) bool {
	ts := rec.Timestamp.UTC()
	if !flt.From.IsZero() && ts.Before(flt.From.UTC()) {
		return false
	}
	if !flt.To.IsZero() && ts.After(flt.To.UTC()) {
		return false
	}
	if strings.TrimSpace(flt.Service) != "" && rec.Service != flt.Service {
		return false
	}
	if strings.TrimSpace(flt.NodeID) != "" && rec.NodeID != flt.NodeID {
		return false
	}
	if strings.TrimSpace(flt.Level) != "" && rec.Severity != flt.Level {
		return false
	}
	return true
}

func openMaybeGzip(path string) (io.Reader, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}
		return gz, func() error { _ = gz.Close(); return f.Close() }, nil
	}
	return f, f.Close, nil
}

func (s *Store) compressAndCleanupLocked(now time.Time) error {
	files, err := filepath.Glob(filepath.Join(s.dir, "ops-*.ndjson"))
	if err != nil {
		return err
	}
	for _, p := range files {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if now.Sub(fi.ModTime()) < 24*time.Hour {
			continue
		}
		if err := compressFile(p); err != nil {
			continue
		}
	}
	gzFiles, err := filepath.Glob(filepath.Join(s.dir, "ops-*.ndjson.gz"))
	if err != nil {
		return err
	}
	for _, p := range gzFiles {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if now.Sub(fi.ModTime()) > s.retention {
			_ = os.Remove(p)
		}
	}
	return nil
}

func compressFile(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	outPath := path + ".gz"
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		_ = gz.Close()
		_ = out.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(path)
}
