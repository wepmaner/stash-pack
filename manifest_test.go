package pack

import (
	"strings"
	"testing"
)

const bridgeManifest = `{
  "id": "hyperhdr-bridge",
  "name": "Подсветка",
  "description": "Discord → HyperHDR",
  "icon": "lightbulb",
  "hue": 45,
  "kind": "exe",
  "entry": "bridge.exe",
  "preserve": ["config.yaml", "token.json", "bridge.log"],
  "exclude": ["*.md"],
  "requirements": [
    "Отсутствие девушки",
    { "label": "Windows 10+", "check": { "type": "os", "min": "10.0.19041" } },
    { "label": "HyperHDR 20+", "check": { "type": "app", "name": "HyperHDR", "exe": "hyperhdr.exe", "min": "20.0" } }
  ],
  "busyWhen": { "process": ["bridge.exe"] },
  "minStash": "0.1.0"
}`

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest([]byte(bridgeManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "hyperhdr-bridge" || m.Kind != KindExe || m.Entry != "bridge.exe" || m.Hue != 45 {
		t.Fatalf("поля разобраны неверно: %+v", m)
	}
	if len(m.Requirements) != 3 {
		t.Fatalf("требований %d, ждали 3", len(m.Requirements))
	}
	if r := m.Requirements[0]; r.Label != "Отсутствие девушки" || r.Check != nil {
		t.Fatalf("строка должна стать требованием без проверки: %+v", r)
	}
	if r := m.Requirements[2]; r.Check == nil || r.Check.Type != "app" || r.Check.Exe != "hyperhdr.exe" || r.Check.Min != "20.0" {
		t.Fatalf("проверка app разобрана неверно: %+v", r.Check)
	}
}

func TestValidate(t *testing.T) {
	bad := map[string]string{
		"id":       `{"id":"Bad ID","name":"x","kind":"files"}`,
		"name":     `{"id":"x","kind":"files"}`,
		"kind":     `{"id":"x","name":"x","kind":"msi"}`,
		"entry":    `{"id":"x","name":"x","kind":"exe"}`,
		"check":    `{"id":"x","name":"x","kind":"files","requirements":[{"label":"A","check":{"type":"magic"}}]}`,
		"label":    `{"id":"x","name":"x","kind":"files","requirements":[{"check":{"type":"os"}}]}`,
		"minStash": `{"id":"x","name":"x","kind":"files","minStash":"soon"}`,
	}
	for field, raw := range bad {
		_, err := ParseManifest([]byte(raw))
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("%s: ждали ошибку про %q, получили %v", field, field, err)
		}
	}
	if _, err := ParseManifest([]byte(`{"id":"x","name":"x","kind":"files","unknown":1}`)); err == nil {
		t.Error("неизвестное поле должно быть ошибкой — скорее всего это опечатка")
	}
}

func TestNormalizeVersion(t *testing.T) {
	for in, want := range map[string]string{"v1.2.3": "1.2.3", "1.2.3": "1.2.3", "v0.10.0-beta.1": "0.10.0-beta.1"} {
		got, err := NormalizeVersion(in)
		if err != nil || got != want {
			t.Errorf("NormalizeVersion(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "latest", "1.2", "v1.x.0"} {
		if _, err := NormalizeVersion(in); err == nil {
			t.Errorf("NormalizeVersion(%q) должна вернуть ошибку", in)
		}
	}
}
