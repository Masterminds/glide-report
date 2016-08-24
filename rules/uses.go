package rules

import (
	"sort"
	"strconv"

	"github.com/Masterminds/glide/cfg"
	"github.com/Masterminds/glide/msg"
	"github.com/Masterminds/semver"
	"github.com/Masterminds/vcs"
)

func usesSemver(name string, dep *cfg.Dependency, lock *cfg.Lock, repo vcs.Repo) {

	refs, err := getAllVcsRefs(repo)
	if err != nil {
		msg.Die("Unable to get refs: %s", err)
	}

	srs := getSemVers(refs)
	var uses bool
	if len(srs) > 0 {
		msg.Puts("%s Dependency provides Semantic Version releases", markC)
		uses = true
	} else {
		msg.Puts("%s Dependency does not provide Semantic Version releases", markD)
	}

	if uses {
		tgs, err := repo.TagsFromCommit(lock.Version)
		if err != nil {
			msg.Die("Unable to get tags from commit: %s", err)
		}

		sort.Sort(sort.Reverse(semver.Collection(srs)))

		if len(tgs) == 0 {
			msg.Puts("%s Using development revision between Semantic Version releases", markX)
		} else {

			tg := tgs[0]
			_, err := semver.NewVersion(tg)
			if err != nil {
				msg.Puts("%s Using non-semantic version (%s) for project supporting Semantic Version", markD, tg)
				return
			}
			i := 0
			var latestV *semver.Version
			var latest bool
			var recent bool
			for _, v := range srs {
				if i == 0 {
					latestV = v
				}
				if tg == v.Original() {
					if i == 0 {
						msg.Puts("%s Using latest release (%s)", markC, tg)
						latest = true
					} else if i > 0 && i < 5 {
						msg.Puts("%s Using recent release (%d behind latest, latest: %s, using: %s)", markD, i, latestV.Original(), tg)
						recent = true
					} else {
						msg.Puts("%s %d releases behind latest release (latest: %s, using: %s)", markX, i, latestV.Original(), tg)
					}

					if v.Prerelease() != "" {
						msg.Puts("%s Using a pre-release version", markX)
					}
				}

				i++
			}

			if !latest {
				c, err := semver.NewConstraint("^" + strconv.FormatInt(latestV.Major(), 10))
				if err != nil {
					msg.Die("Unable to generate SemVer constraint: %s", err)
				}

				sv, err := semver.NewVersion(tg)
				if err != nil {
					msg.Die("Unable to generate SemVer on %s: %s", tg, err)
				}
				if !c.Check(sv) {
					if recent {
						msg.Puts("%s Not using latest Major Semantic Version", markD)
					} else {
						msg.Puts("%s Not using latest Major Semantic Version", markX)
					}
				} else {
					msg.Puts("%s Using latest Major Semantic Version", markC)
				}
			}
		}

	}

}
