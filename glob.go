package pack

import (
	"path"
	"strings"
)

// Match сверяет путь внутри пакета (через «/») с маской из stash.json.
// Как path.Match, плюс «**» — любое число каталогов, включая ноль.
// «dir/**» совпадает и с самим каталогом dir. Регистр не важен: это Windows.
func Match(pattern, p string) bool {
	return matchSegs(split(strings.ToLower(pattern)), split(strings.ToLower(p)))
}

// MatchAny — совпадает ли путь хотя бы с одной маской.
func MatchAny(patterns []string, p string) bool {
	for _, pat := range patterns {
		if Match(pat, p) {
			return true
		}
	}
	return false
}

func split(s string) []string {
	s = strings.Trim(strings.ReplaceAll(s, `\`, "/"), "/")
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			for i := 0; i <= len(segs); i++ {
				if matchSegs(rest, segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}
