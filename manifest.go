// Пакет pack описывает формат пакета Stash (stash.json + files.json + package.zip)
// и собирает его из папки со сборкой. Этот же код потом читает ядро Stash.
package pack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Kind string

const (
	KindExe   Kind = "exe"
	KindFiles Kind = "files"
	// KindChrome — распакованное расширение Chrome: набор файлов с manifest.json
	// в корне, который пользователь один раз загружает в chrome://extensions.
	KindChrome Kind = "chrome"
)

// Manifest — stash.json. Лежит в корне репозитория приложения; в релиз уходит
// с заполненной версией.
type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Icon — имя иконки Lucide для каталога, Hue — оттенок плитки (0–360).
	Icon string `json:"icon,omitempty"`
	Hue  int    `json:"hue,omitempty"`
	Kind Kind   `json:"kind"`
	// Entry — что запускать и за каким процессом следить (для exe).
	Entry string `json:"entry,omitempty"`
	// Preserve — файлы пользователя. Если такой файл есть в пакете, он ставится
	// только при первой установке и дальше не заменяется и не удаляется.
	Preserve []string `json:"preserve,omitempty"`
	// Exclude — что из папки сборки не класть в пакет.
	Exclude      []string      `json:"exclude,omitempty"`
	Requirements []Requirement `json:"requirements,omitempty"`
	BusyWhen     *BusyWhen     `json:"busyWhen,omitempty"`
	MinStash     string        `json:"minStash,omitempty"`
	// Version проставляет stash-pack из тега релиза.
	Version string `json:"version,omitempty"`
}

// BusyWhen — пока запущены эти процессы, файлы приложения не трогаем.
type BusyWhen struct {
	Process []string `json:"process"`
}

// Requirement — строка (просто текст) или объект с автопроверкой.
type Requirement struct {
	Label string `json:"label"`
	Check *Check `json:"check,omitempty"`
}

type Check struct {
	Type  string `json:"type"` // app | os | registry | file | manual
	Name  string `json:"name,omitempty"`
	Exe   string `json:"exe,omitempty"`
	Min   string `json:"min,omitempty"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
	Path  string `json:"path,omitempty"`
}

func (r *Requirement) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &r.Label)
	}
	type plain Requirement
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode((*plain)(r))
}

var (
	idRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	checks    = map[string]bool{"app": true, "os": true, "registry": true, "file": true, "manual": true}
)

// ParseManifest разбирает и проверяет stash.json. Неизвестные поля — ошибка:
// опечатка в «preserve» не должна молча стереть файлы пользователя.
func ParseManifest(b []byte) (*Manifest, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var m Manifest
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("stash.json: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) Validate() error {
	var errs []error
	bad := func(field, format string, a ...any) {
		errs = append(errs, fmt.Errorf("stash.json: %s: %s", field, fmt.Sprintf(format, a...)))
	}
	if !idRe.MatchString(m.ID) {
		bad("id", "%q — только a-z, 0-9 и дефис", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		bad("name", "не задано")
	}
	switch m.Kind {
	case KindExe:
		if m.Entry == "" {
			bad("entry", "для kind=exe укажите, какой exe запускать")
		}
	case KindFiles, KindChrome:
	default:
		bad("kind", "%q — поддерживаются exe, files и chrome", m.Kind)
	}
	if m.Hue < 0 || m.Hue > 360 {
		bad("hue", "%d — нужен оттенок 0–360", m.Hue)
	}
	for i, r := range m.Requirements {
		if strings.TrimSpace(r.Label) == "" {
			bad("requirements", "#%d: нет label", i+1)
		}
		if r.Check != nil && !checks[r.Check.Type] {
			bad("requirements", "#%d: неизвестный check.type %q", i+1, r.Check.Type)
		}
	}
	if m.MinStash != "" && !versionRe.MatchString(m.MinStash) {
		bad("minStash", "%q — нужна версия вида 1.2.3", m.MinStash)
	}
	if m.Version != "" && !versionRe.MatchString(m.Version) {
		bad("version", "%q — нужна версия вида 1.2.3", m.Version)
	}
	return errors.Join(errs...)
}

// NormalizeVersion превращает тег «v1.2.3» в «1.2.3» и проверяет формат.
func NormalizeVersion(tag string) (string, error) {
	v := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if !versionRe.MatchString(v) {
		return "", fmt.Errorf("версия %q: нужен тег вида v1.2.3", tag)
	}
	return v, nil
}
