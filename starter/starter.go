// Package starter holds the starter rules templates in every language of the
// contract (FTR.HMR.DEV-0003 R24): Hammurapi puts them into an empty
// specification repository, and the tests keep them within the contract.
package starter

import (
	"embed"
	"io/fs"
	"path"

	"github.com/GreenOnGrey/speclint/contract"
)

//go:embed ru en
var files embed.FS

// Files are the starter rules in lang, by path from the root of a repository:
// rules/<area>/template.md and rules/<area>/fix-template.md.
func Files(lang contract.Lang) map[string][]byte {
	out := map[string][]byte{}
	root := string(lang)
	_ = fs.WalkDir(files, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := relPath(root, p)
		out[rel] = b
		return nil
	})
	return out
}

func relPath(root, p string) (string, bool) {
	if len(p) > len(root)+1 && p[:len(root)+1] == root+"/" {
		return p[len(root)+1:], true
	}
	return path.Clean(p), false
}
