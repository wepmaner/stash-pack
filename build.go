package pack

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Имена ассетов релиза.
const (
	ManifestName = "stash.json"
	FilesName    = "files.json"
	PackageName  = "package.zip"
)

// FileList — files.json: эталон для проверки, починки и дельта-обновлений.
type FileList struct {
	Version string  `json:"version"`
	Files   []Entry `json:"files"`
}

type Entry struct {
	Path   string `json:"path"` // через «/», относительно корня приложения
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Options struct {
	Dir      string // папка со сборкой
	Out      string // куда положить три ассета
	Manifest *Manifest
	Version  string // тег релиза: v1.2.3 или 1.2.3
}

type Result struct {
	Files   int
	Size    int64 // сумма размеров файлов
	ZipSize int64
}

// fixedTime — время файлов в zip. Одинаковое, чтобы одна и та же сборка давала
// одинаковый архив (и одинаковые хэши у всех, кто соберёт релиз заново).
var fixedTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// Уже сжатые форматы кладём как есть: второй раз они не сожмутся, а распаковка медленнее.
var stored = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true,
	".zip": true, ".7z": true, ".gz": true, ".xz": true, ".mp4": true, ".mp3": true, ".ogg": true,
	".woff": true, ".woff2": true,
}

// Build собирает пакет: stash.json (с версией), files.json и package.zip.
func Build(o Options) (*Result, error) {
	version, err := NormalizeVersion(o.Version)
	if err != nil {
		return nil, err
	}
	m := *o.Manifest
	m.Version = version
	if err := m.Validate(); err != nil {
		return nil, err
	}

	entries, err := collect(o.Dir, o.Out, m.Exclude)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("в пакете нет файлов — проверьте dir и exclude")
	}
	if m.Kind == KindExe && !hasPath(entries, m.Entry) {
		return nil, fmt.Errorf("entry %q нет в сборке %s", m.Entry, o.Dir)
	}

	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return nil, err
	}
	res := &Result{Files: len(entries)}
	zipPath := filepath.Join(o.Out, PackageName)
	if err := writeZip(zipPath, o.Dir, entries); err != nil {
		return nil, err
	}
	for _, e := range entries {
		res.Size += e.Size
	}
	if st, err := os.Stat(zipPath); err == nil {
		res.ZipSize = st.Size()
	}
	if err := writeJSON(filepath.Join(o.Out, FilesName), FileList{Version: version, Files: entries}); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(o.Out, ManifestName), m); err != nil {
		return nil, err
	}
	return res, nil
}

func collect(dir, out string, exclude []string) ([]Entry, error) {
	absOut, _ := filepath.Abs(out)
	var entries []Entry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if abs, _ := filepath.Abs(p); d.IsDir() && abs == absOut {
			return filepath.SkipDir // собственные артефакты не пакуем
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if MatchAny(exclude, rel) {
			return nil
		}
		sum, size, err := hashFile(p)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{Path: rel, Size: size, SHA256: sum})
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, err
}

func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func writeZip(dst, dir string, entries []Entry) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		method := zip.Deflate
		if stored[strings.ToLower(path.Ext(e.Path))] {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.Path, Method: method, Modified: fixedTime})
		if err != nil {
			f.Close()
			return err
		}
		src, err := os.Open(filepath.Join(dir, filepath.FromSlash(e.Path)))
		if err != nil {
			f.Close()
			return err
		}
		_, err = io.Copy(w, src)
		src.Close()
		if err != nil {
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func writeJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

func hasPath(entries []Entry, p string) bool {
	p = strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
	for _, e := range entries {
		if strings.ToLower(e.Path) == p {
			return true
		}
	}
	return false
}
