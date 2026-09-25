package pack

import (
	"archive/zip"
	"bytes"
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
	ImageName    = "icon.png" // необязательный: иконка из Manifest.Image
)

// maxImage — иконка для каталога, а не обои: больше не нужно.
const maxImage = 512 << 10

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
	// ManifestDir — папка stash.json: от неё считается путь Manifest.Image.
	ManifestDir string
}

type Result struct {
	Files   int
	Size    int64 // сумма размеров файлов
	ZipSize int64
	Image   bool // положили icon.png
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
	if m.Kind == KindChrome {
		if err := checkChromeManifest(o.Dir, version); err != nil {
			return nil, err
		}
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
	if m.Image != "" {
		if err := copyImage(filepath.Join(o.ManifestDir, filepath.FromSlash(m.Image)), filepath.Join(o.Out, ImageName)); err != nil {
			return nil, err
		}
		res.Image = true
		m.Image = ""
	}
	if err := writeJSON(filepath.Join(o.Out, ManifestName), m); err != nil {
		return nil, err
	}
	return res, nil
}

func copyImage(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("image: %w", err)
	}
	if !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		return fmt.Errorf("image: %s — нужен PNG", src)
	}
	if len(b) > maxImage {
		return fmt.Errorf("image: %s — %d КБ, нужно не больше %d КБ", src, len(b)>>10, maxImage>>10)
	}
	return os.WriteFile(dst, b, 0o644)
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

// checkChromeManifest: у расширения в корне пакета есть manifest.json, и его
// version совпадает с версией релиза — по ней расширение замечает, что Stash
// обновил его файлы, а Chrome считает загруженной старую версию.
func checkChromeManifest(dir, version string) error {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("kind=chrome: в корне сборки нет manifest.json (%w)", err)
	}
	var cm struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &cm); err != nil {
		return fmt.Errorf("kind=chrome: manifest.json: %w", err)
	}
	if cm.Version != version {
		return fmt.Errorf("kind=chrome: version в manifest.json %q, а релиз %q — проставьте версию из тега при сборке", cm.Version, version)
	}
	return nil
}
