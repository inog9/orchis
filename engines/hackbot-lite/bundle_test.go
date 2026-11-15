package pentestharnes_test

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"

	pentestharnes "github.com/r1pu5u/pentest-harnes"
)

func TestBundledMarkdownReferencesResolve(t *testing.T) {
	source := pentestharnes.Skills()
	links := regexp.MustCompile(`\[[^\]]+\]\(([^)\s]+)\)`)
	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(name, ".md") {
			return err
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		for _, match := range links.FindAllStringSubmatch(string(data), -1) {
			target := match[1]
			if strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "#") {
				continue
			}
			file, _, _ := strings.Cut(target, "#")
			resolved := path.Join(path.Dir(name), file)
			if _, err := fs.Stat(source, resolved); err != nil {
				t.Errorf("%s has an unresolved bundled reference %q: %v", name, target, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
