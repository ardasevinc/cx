package projects

import (
	"path/filepath"
	"strings"

	"github.com/ardasevinc/cx/internal/sessions"
)

type Scope struct {
	CWD  string
	Root Root
}

func NewScope(cwd string, opts Options) Scope {
	root := ClassifyCWD(cwd, opts)
	return Scope{CWD: absolutePath(cwd), Root: root}
}

func FilterSessionsByCWD(items []sessions.Session, cwd string, opts Options) []sessions.Session {
	scope := NewScope(cwd, opts)
	filtered := make([]sessions.Session, 0, len(items))
	for _, item := range items {
		if scope.Matches(item.CWD, opts) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (s Scope) Matches(cwd string, opts Options) bool {
	if s.CWD == "" || cwd == "" {
		return false
	}
	target := absolutePath(s.CWD)
	candidate := absolutePath(cwd)
	if samePath(target, candidate) {
		return true
	}

	candidateRoot := ClassifyCWD(candidate, opts)
	if s.Root.Kind == KindGit && candidateRoot.Kind == KindGit && samePath(s.Root.Dir, candidateRoot.Dir) {
		return true
	}
	if s.Root.Kind == KindCWD && candidateRoot.Kind == KindCWD && samePath(s.Root.Dir, candidateRoot.Dir) {
		return true
	}
	if s.Root.Kind == KindChat && candidateRoot.Kind == KindChat {
		return true
	}
	if s.Root.Kind == KindHome || s.Root.Kind == KindUnknown || s.Root.Kind == KindMissing {
		return false
	}
	if candidateRoot.Kind == KindHome || candidateRoot.Kind == KindUnknown || candidateRoot.Kind == KindMissing {
		return false
	}
	return pathContains(target, candidate)
}

func pathContains(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if samePath(parent, child) {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	return err == nil && isRelativeInside(rel)
}

func isRelativeInside(rel string) bool {
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
