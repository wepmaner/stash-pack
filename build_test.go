package pack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func testManifest(t *testing.T) *Manifest {
	m, err := ParseManifest([]byte(`{"id":"hyperhdr-bridge","name":"Подсветка","kind":"exe","entry":"bridge.exe",
		"preserve":["config.yaml"],"exclude":["*.md","logs/**"]}`))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestBuild(t *testing.T) {
	src, out := t.TempDir(), t.TempDir()
	writeTree(t, src, map[string]string{
		"bridge.exe":          "MZ fake exe",
		"rpcprobe.exe":        "MZ probe",
		"config.example.yaml": "log_level: info\n",
		"README.md":           "не должно попасть",
		"logs/old.log":        "тоже",
		"assets/icon.png":     "png",
	})

	res, err := Build(Options{Dir: src, Out: out, Manifest: testManifest(t), Version: "v1.2.0"})
	if err != nil {
		t.Fatal(err)
	}

	var fl FileList
	readJSON(t, filepath.Join(out, FilesName), &fl)
	if fl.Version != "1.2.0" {
		t.Fatalf("версия в files.json: %q", fl.Version)
	}
	var paths []string
	for _, f := range fl.Files {
		paths = append(paths, f.Path)
	}
	if got := strings.Join(paths, ","); got != "assets/icon.png,bridge.exe,config.example.yaml,rpcprobe.exe" {
		t.Fatalf("состав пакета: %s", got)
	}
	if res.Files != 4 {
		t.Fatalf("Result.Files = %d", res.Files)
	}

	// Каждый файл в zip совпадает с files.json по размеру и sha256.
	zr, err := zip.OpenReader(filepath.Join(out, PackageName))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != len(fl.Files) {
		t.Fatalf("в zip %d файлов, в files.json %d", len(zr.File), len(fl.Files))
	}
	for i, zf := range zr.File {
		rc, _ := zf.Open()
		body, _ := io.ReadAll(rc)
		rc.Close()
		sum := sha256.Sum256(body)
		want := fl.Files[i]
		if zf.Name != want.Path || int64(len(body)) != want.Size || hex.EncodeToString(sum[:]) != want.SHA256 {
			t.Fatalf("zip[%d] %s не совпадает с files.json %+v", i, zf.Name, want)
		}
	}
	if zr.File[0].Method != zip.Store {
		t.Error("png уже сжат — его надо класть без сжатия")
	}

	var m Manifest
	readJSON(t, filepath.Join(out, ManifestName), &m)
	if m.Version != "1.2.0" || m.ID != "hyperhdr-bridge" {
		t.Fatalf("stash.json в релизе: %+v", m)
	}
}

func TestBuildDeterministic(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"bridge.exe": "MZ", "a/b.txt": "b", "c.txt": "c"})
	var zips [][]byte
	for i := 0; i < 2; i++ {
		out := t.TempDir()
		if _, err := Build(Options{Dir: src, Out: out, Manifest: testManifest(t), Version: "1.0.0"}); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(out, PackageName))
		zips = append(zips, b)
	}
	if !bytes.Equal(zips[0], zips[1]) {
		t.Fatal("одна и та же папка должна давать байт-в-байт одинаковый zip")
	}
}

func TestBuildErrors(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"other.exe": "MZ"})
	_, err := Build(Options{Dir: src, Out: t.TempDir(), Manifest: testManifest(t), Version: "1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "bridge.exe") {
		t.Fatalf("нет entry в сборке — ждали ошибку, получили %v", err)
	}
	_, err = Build(Options{Dir: src, Out: t.TempDir(), Manifest: testManifest(t), Version: "latest"})
	if err == nil {
		t.Fatal("кривая версия должна быть ошибкой")
	}
	empty := t.TempDir()
	writeTree(t, empty, map[string]string{"README.md": "x"})
	m, _ := ParseManifest([]byte(`{"id":"x","name":"x","kind":"files","exclude":["*.md"]}`))
	if _, err = Build(Options{Dir: empty, Out: t.TempDir(), Manifest: m, Version: "1.0.0"}); err == nil {
		t.Fatal("пустой пакет должен быть ошибкой")
	}
}

func TestBuildSkipsOutDirInsideSource(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"bridge.exe": "MZ"})
	out := filepath.Join(src, "stash-out")
	if _, err := Build(Options{Dir: src, Out: out, Manifest: testManifest(t), Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	// Повторная сборка не должна упаковать собственные прошлые артефакты.
	res, err := Build(Options{Dir: src, Out: out, Manifest: testManifest(t), Version: "1.0.0"})
	if err != nil || res.Files != 1 {
		t.Fatalf("ждали 1 файл, получили %+v, %v", res, err)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func TestBuildChrome(t *testing.T) {
	m, err := ParseManifest([]byte(`{"id":"ext","name":"Расширение","kind":"chrome"}`))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		files map[string]string
		err   string
	}{
		{map[string]string{"manifest.json": `{"version":"1.4.0"}`, "popup.js": "x"}, ""},
		{map[string]string{"popup.js": "x"}, "нет manifest.json"},
		{map[string]string{"manifest.json": `{"version":"1.3"}`}, `"1.3", а релиз "1.4.0"`},
	}
	for _, c := range cases {
		src := t.TempDir()
		writeTree(t, src, c.files)
		_, err := Build(Options{Dir: src, Out: t.TempDir(), Manifest: m, Version: "v1.4.0"})
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Fatalf("%v: ждали %q, получили %v", c.files, c.err, err)
		}
	}
}
