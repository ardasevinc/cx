package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ModulePath = "github.com/ardasevinc/cx/cmd/cx"
	RepoURL    = "https://github.com/ardasevinc/cx.git"
)

type Status int

const (
	Unknown Status = iota
	Current
	Outdated
	Ahead
)

type Check struct {
	Current string
	Latest  string
	Status  Status
}

func CheckLatest(ctx context.Context, current string) (Check, error) {
	latest, err := LatestTag(ctx)
	if err != nil {
		return Check{}, err
	}
	return Check{
		Current: current,
		Latest:  latest,
		Status:  CompareVersions(current, latest),
	}, nil
}

func LatestTag(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", RepoURL)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read cx tags: %w", err)
	}
	tag, ok := LatestTagFromLsRemote(output)
	if !ok {
		return "", fmt.Errorf("no version tags found")
	}
	return tag, nil
}

func Install(ctx context.Context, tag string, stdout, stderr *os.File) error {
	if strings.TrimSpace(tag) == "" {
		tag = "latest"
	}
	cmd := exec.CommandContext(ctx, "go", "install", ModulePath+"@"+tag)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func LatestTagFromLsRemote(output []byte) (string, bool) {
	var versions []string
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			continue
		}
		ref := string(fields[1])
		tag := strings.TrimPrefix(ref, "refs/tags/")
		if _, ok := parseVersion(tag); ok {
			versions = append(versions, tag)
		}
	}
	if len(versions) == 0 {
		return "", false
	}
	sort.Slice(versions, func(i, j int) bool {
		return CompareVersions(versions[i], versions[j]) == Ahead
	})
	return versions[0], true
}

func CompareVersions(current, latest string) Status {
	cur, curOK := parseVersion(current)
	next, nextOK := parseVersion(latest)
	if !curOK || !nextOK {
		return Unknown
	}
	for i := range cur {
		switch {
		case cur[i] < next[i]:
			return Outdated
		case cur[i] > next[i]:
			return Ahead
		}
	}
	return Current
}

func StatusText(status Status) string {
	switch status {
	case Current:
		return "up to date"
	case Outdated:
		return "update available"
	case Ahead:
		return "ahead of latest tag"
	default:
		return "unknown"
	}
}

func parseVersion(value string) ([3]int, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var version [3]int
	for i, part := range parts {
		if part == "" || strings.ContainsFunc(part, func(r rune) bool { return r < '0' || r > '9' }) {
			return [3]int{}, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return [3]int{}, false
		}
		version[i] = n
	}
	return version, true
}
