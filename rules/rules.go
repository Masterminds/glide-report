package rules

import (
	"github.com/Masterminds/glide/cfg"
	"github.com/Masterminds/glide/msg"
	"github.com/Masterminds/semver"
	"github.com/Masterminds/vcs"
)

var markX = msg.Color(msg.Red, "X")
var markC = msg.Color(msg.Green, "✓")
var markD = msg.Color(msg.Yellow, "●")

var cbs []cfunc

func init() {
	cbs = []cfunc{
		usesSemver,
		howOld,
	}
}

func Setup(noColor bool) {
	if noColor {
		markX = "X"
		markC = "✓"
		markD = "●"
	}
}

func Rules() []cfunc {
	return cbs
}

type cfunc func(name string, dep *cfg.Dependency, lock *cfg.Lock, repo vcs.Repo)

func getSemVers(refs []string) []*semver.Version {
	sv := []*semver.Version{}
	for _, r := range refs {
		v, err := semver.NewVersion(r)
		if err == nil {
			sv = append(sv, v)
		}
	}

	return sv
}

func getAllVcsRefs(repo vcs.Repo) ([]string, error) {
	tags, err := repo.Tags()
	if err != nil {
		return []string{}, err
	}

	return tags, nil
}
