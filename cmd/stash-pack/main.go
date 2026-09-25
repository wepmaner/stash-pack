// stash-pack собирает пакет Stash из папки со сборкой приложения:
//
//	stash-pack -dir dist -version v1.2.0 -out stash-out
//
// Рядом появятся stash.json, files.json и package.zip — их и прикрепляют к релизу.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	pack "github.com/wepmaner/stash-pack"
)

func main() {
	dir := flag.String("dir", "dist", "папка со сборкой")
	manifest := flag.String("manifest", pack.ManifestName, "путь к stash.json из репозитория")
	version := flag.String("version", os.Getenv("VERSION"), "версия или тег релиза (v1.2.3); по умолчанию $VERSION")
	out := flag.String("out", "stash-out", "куда положить ассеты релиза")
	flag.Parse()

	if err := run(*dir, *manifest, *version, *out); err != nil {
		fmt.Fprintln(os.Stderr, "stash-pack:", err)
		os.Exit(1)
	}
}

func run(dir, manifestPath, version, out string) error {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	m, err := pack.ParseManifest(raw)
	if err != nil {
		return err
	}
	res, err := pack.Build(pack.Options{Dir: dir, Out: out, Manifest: m, Version: version})
	if err != nil {
		return err
	}
	fmt.Printf("%s %s: %d файлов, %s → package.zip %s\n", m.ID, version, res.Files, size(res.Size), size(res.ZipSize))
	for _, name := range []string{pack.ManifestName, pack.FilesName, pack.PackageName} {
		fmt.Println("  ", filepath.Join(out, name))
	}
	return nil
}

func size(b int64) string {
	if b >= 1<<20 {
		return fmt.Sprintf("%.1f МБ", float64(b)/(1<<20))
	}
	return fmt.Sprintf("%d КБ", (b+1023)/1024)
}
