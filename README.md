# stash-pack

Упаковщик приложений для Stash — приватного каталога программ. Превращает папку
со сборкой в три ассета релиза:

| Файл | Что внутри |
|---|---|
| `stash.json` | описание приложения из репозитория + версия из тега |
| `files.json` | каждый файл: путь, размер, sha256 — эталон для проверки и починки |
| `package.zip` | сами файлы; каждый сжат отдельно, поэтому по HTTP Range можно достать один |

```
go run github.com/wepmaner/stash-pack/cmd/stash-pack@latest \
  -dir dist -manifest stash.json -version v1.2.0 -out stash-out
```

## stash.json

```jsonc
{
  "id": "hyperhdr-bridge",          // a-z, 0-9, дефис
  "name": "Подсветка",
  "description": "…",
  "icon": "lightbulb", "hue": 45,   // иконка Lucide и оттенок плитки
  "kind": "exe",                    // exe | files | chrome (manifest.json в корне, version = тег)
  "image": "assets/icon.png",       // своя иконка (PNG ≤ 512 КБ, путь от stash.json) → icon.png в релизе
  "data": ["${APPDATA}\MyApp"],       // данные вне папки программы — удаляются только «вместе с данными»
  "entry": "bridge.exe",            // для exe: что запускать
  "preserve": ["config.yaml"],      // файлы пользователя: не заменять, не удалять
  "exclude": ["*.md", "logs/**"],   // что не класть в пакет
  "requirements": [
    "Просто текст",
    { "label": "Windows 10+", "check": { "type": "os", "min": "10.0.19041" } },
    { "label": "OBS 30+", "check": { "type": "app", "name": "OBS Studio", "exe": "obs64.exe", "min": "30.0" } }
  ],
  "busyWhen": { "process": ["obs64.exe"] },
  "minStash": "0.1.0"
}
```

- Неизвестное поле — ошибка: опечатка в `preserve` не должна молча стереть файлы пользователя.
- Файл из `preserve`, который есть в пакете, ставится только при первой установке.
- Одна и та же сборка даёт байт-в-байт одинаковый `package.zip`.

Тесты: `go test ./...`
